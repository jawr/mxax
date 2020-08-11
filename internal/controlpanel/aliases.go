package controlpanel

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
	"github.com/jawr/mxax/internal/account"
	"github.com/jawr/mxax/internal/logger"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

func (s *Site) getPostManageAlias() (*route, error) {
	r := &route{
		path:    "/alias/manage/:hash",
		methods: []string{"GET", "POST"},
	}

	// setup template
	tmpl, err := s.loadTemplate("templates/pages/alias.html")
	if err != nil {
		return r, err
	}

	type Destination struct {
		account.Destination
		Aliases int
		HID     string
	}

	// definte template data
	type data struct {
		Route string

		Alias                account.Alias
		Domain               account.Domain
		Destinations         []account.Destination
		ExistingDestinations []Destination

		Errors FormErrors

		// stream
		Entries []logger.Entry

		// stats
		Labels        []string
		InboundSend   []int
		InboundBounce []int
		InboundReject []int
	}

	// actual handler
	r.h = func(tx pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		d := data{
			Route:  "aliases",
			Errors: newFormErrors(),
		}

		ids := s.idHasher.Decode(ps.ByName("hash"))
		if len(ids) != 1 {
			return errors.New("No id found")
		}

		aliasID := ids[0]

		// get alias
		err := account.GetAlias(
			req.Context(),
			tx,
			&d.Alias,
			aliasID,
		)
		if err != nil {
			return errors.WithMessage(err, "GetAlias")
		}

		// once we have the alias we can do some work with it
		if req.Method == "POST" {

			destinationID, err := strconv.Atoi(req.FormValue("destination"))
			if err != nil {
				return errors.WithMessage(err, "Atoi destination")
			}

			// validate that destination belongs to this account
			var destination account.Destination
			err = account.GetDestinationByID(
				req.Context(),
				tx,
				&destination,
				destinationID,
			)
			if err != nil {
				return errors.WithMessage(err, "Validate Destination")
			}

			err = account.CreateAliasDestination(
				req.Context(),
				tx,
				d.Alias.ID,
				destinationID,
			)
			if err != nil {
				log.Printf("Error inserting alias_destination (%d,%d): %s", aliasID, destinationID, err)
				d.Errors.Add(
					"",
					"Unable to attach destination to alias. Please contact support",
				)
			}
		}

		// get domain
		err = account.GetDomainByID(
			req.Context(),
			tx,
			&d.Domain,
			d.Alias.DomainID,
		)
		if err != nil {
			return errors.WithMessage(err, "GetDomainByID")
		}

		// select destinations
		err = pgxscan.Select(
			req.Context(),
			tx,
			&d.Destinations,
			`
			SELECT * 
			FROM destinations AS d 
			WHERE 
				id NOT IN (
					SELECT destination_id 
					FROM alias_destinations
					WHERE 
						alias_id = $1
						AND deleted_at IS NULL
				)
				AND deleted_at IS NULL
			ORDER BY address
			`,
			d.Alias.ID,
		)
		if err != nil {
			return errors.WithMessage(err, "Select destinations")
		}

		// get existing destinations
		err = pgxscan.Select(
			req.Context(),
			tx,
			&d.ExistingDestinations,
			`
			SELECT 
				d.*, 
				COALESCE(COUNT(ad.*) FILTER (
					WHERE ad.deleted_at IS NULL
				), 0) AS aliases
			FROM destinations AS d
				JOIN alias_destinations AS ad ON d.id = ad.destination_id
			WHERE d.deleted_at IS NULL
			AND ad.deleted_at IS NULL
			AND ad.alias_id = $1
			GROUP BY d.id
			`,
			d.Alias.ID,
		)
		if err != nil {
			return err
		}

		for idx := range d.ExistingDestinations {
			d.ExistingDestinations[idx].HID, err = s.idHasher.Encode([]int{
				d.Alias.ID,
				d.ExistingDestinations[idx].ID,
			})
			if err != nil {
				return err
			}
		}

		// get forward entries
		err = pgxscan.Select(
			req.Context(),
			tx,
			&d.Entries,
			`
                SELECT                                                                 
					*
                FROM logs
                WHERE
                    time > NOW() - INTERVAL '48 HOURS'
					AND alias_id = $1
                ORDER BY time DESC
			`,
			aliasID,
		)
		if err != nil {
			return err
		}

		// handle stats
		err = pgxscan.Select(
			req.Context(),
			tx,
			&d.Labels,
			`
			SELECT date_trunc('hour', i)::text  FROM 
				generate_series(
					NOW() - INTERVAL '24 HOURS',
					NOW(),
					INTERVAL '1 HOUR'
			) AS t(i)
			`,
		)
		if err != nil {
			return errors.WithMessage(err, "Select Labels")
		}

		err = pgxscan.Select(
			req.Context(),
			tx,
			&d.InboundSend,
			`
			WITH series AS (
				SELECT date_trunc(
					'hour',
					generate_series(
						NOW() - INTERVAL '24 HOURS',
						NOW(),
						INTERVAL '1 HOUR'
					)
				) AS hour
			), metrics AS (
				SELECT
					date_trunc('hour', l.time) AS hour,
					COUNT(l.*) AS cnt
				FROM logs AS l
					JOIN domains AS d ON l.domain_id = d.id
				WHERE
					time > NOW() - INTERVAL '24 HOURS'
					AND l.etype = $1
					AND l.alias_id = $2
				GROUP BY 1
				ORDER BY 1
			)
			SELECT
				COALESCE(SUM(metrics.cnt), 0)
				
			FROM series
				LEFT JOIN metrics ON series.hour = metrics.hour

			GROUP BY series.hour
			ORDER BY series.hour
			`,
			logger.EntryTypeSend,
			aliasID,
		)
		if err != nil {
			return errors.Wrap(err, "Select EntryTypeSend")
		}

		err = pgxscan.Select(
			req.Context(),
			tx,
			&d.InboundBounce,
			`
			WITH series AS (
				SELECT date_trunc(
					'hour',
					generate_series(
						NOW() - INTERVAL '24 HOURS',
						NOW(),
						INTERVAL '1 HOUR'
					)
				) AS hour
			), metrics AS (
				SELECT
					date_trunc('hour', l.time) AS hour,
					COUNT(l.*) AS cnt
				FROM logs AS l
					JOIN domains AS d ON l.domain_id = d.id
				WHERE
					time > NOW() - INTERVAL '24 HOURS'
					AND l.etype = $1
					AND l.alias_id = $2
				GROUP BY 1
				ORDER BY 1
			)
			SELECT
				COALESCE(SUM(metrics.cnt), 0)
				
			FROM series
				LEFT JOIN metrics ON series.hour = metrics.hour

			GROUP BY series.hour
			ORDER BY series.hour
			`,
			logger.EntryTypeBounce,
			aliasID,
		)
		if err != nil {
			return errors.Wrap(err, "Select EntryTypeBounce")
		}

		err = pgxscan.Select(
			req.Context(),
			tx,
			&d.InboundReject,
			`
			WITH series AS (
				SELECT date_trunc(
					'hour',
					generate_series(
						NOW() - INTERVAL '24 HOURS',
						NOW(),
						INTERVAL '1 HOUR'
					)
				) AS hour
			), metrics AS (
				SELECT
					date_trunc('hour', l.time) AS hour,
					COUNT(l.*) AS cnt
				FROM logs AS l
					JOIN domains AS d ON l.domain_id = d.id
				WHERE
					time > NOW() - INTERVAL '24 HOURS'
					AND l.etype = $1
					AND l.alias_id = $2
				GROUP BY 1
				ORDER BY 1
			)
			SELECT
				COALESCE(SUM(metrics.cnt), 0)
				
			FROM series
				LEFT JOIN metrics ON series.hour = metrics.hour

			GROUP BY series.hour
			ORDER BY series.hour
			`,
			logger.EntryTypeReject,
			aliasID,
		)
		if err != nil {
			return errors.Wrap(err, "Select EntryTypeReject")
		}

		s.renderTemplate(w, tmpl, r, d)

		return nil
	}

	return r, nil
}

func (s *Site) getDeleteAliasDestination() (*route, error) {
	r := &route{
		path:    "/alias/destination/delete/:hash",
		methods: []string{"GET"},
	}

	// actual handler
	r.h = s.confirmAction(func(tx pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		ids := s.idHasher.Decode(ps.ByName("hash"))
		if len(ids) != 2 {
			return errors.New("No id found")
		}

		aliasID := ids[0]
		destinationID := ids[1]

		var alias account.Alias
		var destination account.Destination

		// get alias
		err := account.GetAlias(
			req.Context(),
			tx,
			&alias,
			aliasID,
		)
		if err != nil {
			return errors.WithMessage(err, "GetAlias")
		}

		// get destination
		err = account.GetDestinationByID(
			req.Context(),
			tx,
			&destination,
			destinationID,
		)
		if err != nil {
			return errors.WithMessage(err, "GetDestinationByID")
		}

		_, err = tx.Exec(
			req.Context(),
			`
			UPDATE alias_destinations 
			SET deleted_at = NOW()
			WHERE alias_id = $1 AND destination_id = $2
			`,
			aliasID,
			destinationID,
		)
		if err != nil {
			return errors.WithMessage(err, "Delete")
		}

		aliasHID, err := s.idHasher.Encode([]int{aliasID})
		if err != nil {
			return err
		}

		http.Redirect(w, req, "/alias/manage/"+aliasHID, http.StatusFound)

		return nil
	})

	return r, nil
}

func (s *Site) getDeleteAlias() (*route, error) {
	r := &route{
		path:    "/alias/delete/:hash",
		methods: []string{"GET"},
	}

	// actual handler
	r.h = s.confirmAction(func(tx pgx.Tx, w http.ResponseWriter, req *http.Request, ps httprouter.Params) error {

		ids := s.idHasher.Decode(ps.ByName("hash"))
		if len(ids) != 1 {
			return errors.New("No id found")
		}

		aliasID := ids[0]

		var alias account.Alias

		// get alias
		err := account.GetAlias(
			req.Context(),
			tx,
			&alias,
			aliasID,
		)
		if err != nil {
			return errors.WithMessage(err, "GetAlias")
		}

		var dom account.Domain

		err = account.GetDomainByID(
			req.Context(),
			tx,
			&dom,
			alias.DomainID,
		)
		if err != nil {
			return errors.WithMessage(err, "GetDomainByID")
		}

		_, err = tx.Exec(
			req.Context(),
			`
			UPDATE alias_destinations 
			SET deleted_at = NOW() 
			WHERE alias_id = $1
			`,
			alias.ID,
		)
		if err != nil {
			return errors.WithMessage(err, "Delete")
		}

		_, err = tx.Exec(
			req.Context(),
			"UPDATE aliases SET deleted_at = NOW() WHERE ID = $1",
			alias.ID,
		)
		if err != nil {
			return errors.WithMessage(err, "Delete")
		}

		http.Redirect(w, req, fmt.Sprintf("/domain/manage/%s", dom.Name), http.StatusFound)

		return nil
	})

	return r, nil
}
