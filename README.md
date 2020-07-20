# ma.ax
easey peasey mail routing


## TODO
The following are needed before beta:

[ ] - Errors; consistent error handling in the smtp package (handlers should not be
  responsible for the eventual SMTP error code/message)
[ ] - Queues; we should make the entire inbound process more efficient by being as
  asynchronous as possible, use an MQ between parts. This raises the question,
  do we want to defer the relayHandler/inbound DATA hook's possible bounces?
[ ] - Frontend; pure HTML interface, or ReactJS SPA?
[ ] - Tracing; we want to be able to turn on tracing so we can discover issues
[ ] - Metrics; ability to alert, discover, etc
