package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/isayme/go-amqp-reconnect/rabbitmq"
	"github.com/jawr/mxax/internal/sender"
	"github.com/jawr/mxax/internal/smtp"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
	"golang.org/x/sync/errgroup"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

type StringSliceFlags []string

func (s StringSliceFlags) String() string {
	return strings.Join(s, ",")
}

func (s *StringSliceFlags) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func run() error {
	var ips, rdnss StringSliceFlags
	var queue string

	flag.Var(&ips, "ips", "List of IP addresses to listen to. Order must match -rdns.")
	flag.Var(&rdnss, "rdns", "List of corresponding rdns. Order must match -ips.")
	flag.StringVar(&queue, "queue", "", "Name of the queue to subscribe to")
	flag.Parse()

	log.Println(ips)
	log.Println(rdnss)

	if flag.NFlag() == 0 {
		flag.PrintDefaults()
		return nil
	}

	if len(ips) == 0 {
		return errors.New("must specify an ip to listen on")
	}

	if len(ips) != len(rdnss) {
		return errors.New("ips and rdns should be of equal length")
	}

	if len(queue) == 0 {
		return errors.New("must specify a queue")
	}

	if _, ok := smtp.Queues[queue]; !ok {
		return errors.New("that queue does not exist")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// setup rabbitmq connection
	rabbitConn, err := rabbitmq.Dial(os.Getenv("MXAX_MQ_URL"))
	if err != nil {
		return errors.WithMessage(err, "rabbitmq.Dial")
	}
	defer rabbitConn.Close()

	// setup logs publisher
	publisher, err := createPublisher(rabbitConn, "")
	if err != nil {
		return errors.WithMessage(err, "createPublisher")
	}
	defer publisher.Close()

	// setup email subscriber
	hostname, err := os.Hostname()
	if err != nil {
		return errors.WithMessage(err, "Hostname")
	}

	emailSubscriber, emailSubscriberCh, err := createSubscriber(rabbitConn, queue, hostname+".sender")
	if err != nil {
		return errors.WithMessage(err, "createSubscriber emails")
	}
	defer emailSubscriber.Close()

	bounceSubscriber, bounceSubscriberCh, err := createSubscriber(rabbitConn, queue, hostname+".sender")
	if err != nil {
		return errors.WithMessage(err, "createSubscriber bounces")
	}
	defer bounceSubscriber.Close()

	log.Println("Connected to MQ...")

	// create our sender
	sndr, err := sender.NewSender(publisher, emailSubscriberCh, bounceSubscriberCh)
	if err != nil {
		return errors.WithMessage(err, "NewSender")
	}

	eg := &errgroup.Group{}

	for idx := range ips {
		ip := ips[idx]
		parts := strings.Split(ip, ":")
		ip = parts[0]

		var bind string
		if len(parts) > 1 {
			bind = parts[1]
		} else {
			bind = ip
		}

		rdns := rdnss[idx]

		// verify we can listen on this ip
		laddr, err := net.ResolveTCPAddr("tcp", bind+":0")
		if err != nil {
			return errors.WithMessagef(err, "ResolveTCPAddr for '%s'", ip)
		}

		// setup dialer
		dialer := net.Dialer{
			Timeout:   time.Second * 10,
			LocalAddr: laddr,
		}

		conn, err := dialer.DialContext(ctx, "tcp", "8.8.8.8:53")
		if err != nil {
			return errors.WithMessagef(err, "Dial using %s", ip)
		}
		conn.Close()

		// test dialer and verify that rdns matches
		if err := verifyRdns(ip, rdns); err != nil {
			return errors.WithMessagef(err, "verifyRdns for '%s' / '%s'", ip, rdns)
		}

		// run using errgroup
		eg.Go(func() error {
			return sndr.Run(ctx, dialer, rdns)
		})
	}

	// signal all runners to run
	sndr.Start()

	if err := eg.Wait(); err != nil {
		log.Printf("ERROR: %s", err)
		return errors.WithMessage(err, "Wait")
	}

	return nil
}

func createSubscriber(conn *rabbitmq.Connection, queueName, name string) (*rabbitmq.Channel, <-chan amqp.Delivery, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, errors.WithMessage(err, "subscriber.Channel")
	}

	if err := ch.Qos(1, 0, false); err != nil {
		return nil, nil, errors.WithMessage(err, "Qos")
	}

	msgs, err := ch.Consume(
		queueName,
		name,
		false, // autoack
		false, // exclusive
		false, // nolocal
		false, // nowait
		nil,
	)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "ch.Consume")
	}

	return ch, msgs, nil
}

func createPublisher(conn *rabbitmq.Connection, queueName string) (*rabbitmq.Channel, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, errors.WithMessage(err, "subscriber.Channel")
	}

	if len(queueName) > 0 {
		_, err = ch.QueueDeclare(
			queueName,
			true,  // durable
			false, // autoDelete
			false, // exclusive
			false, // noWait
			nil,
		)
		if err != nil {
			return nil, errors.WithMessage(err, "QueueDeclare")
		}
	}

	return ch, nil
}

func verifyRdns(ip, rdns string) error {
	ips, err := net.LookupIP(rdns)
	if err != nil {
		return errors.WithMessage(err, "LookupIP")
	}
	if len(ips) != 1 {
		return errors.New("too many results found")
	}
	if ips[0].String() != ip {
		return errors.Errorf("lookup '%s' found '%s' expected '%s'", rdns, ips[0].String(), ip)
	}
	return nil
}
