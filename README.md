# ma.ax
easey peasey mail routing


## TODO
The following are needed before beta:

- [ ] Return-Path; detect Reply-To header and fallback to From
- [x] Errors; consistent error handling in the smtp package (handlers should not be
  responsible for the eventual SMTP error code/message)
- [ ] Queues; we should make the entire inbound process more efficient by being as
  asynchronous as possible, use an MQ between parts. This raises the question,
  do we want to defer the relayHandler/inbound DATA hook's possible bounces?
- [ ] Frontend; pure HTML interface, or ReactJS SPA?
- [ ] Logging; github.com/uber-go/zap
- [ ] Metrics; ability to alert, discover, etc
- [ ] Rework the handlers, very clunky at the moment

## Features
Some ideas

- Aliases should have a settable order, or the ability to define a catchall after 
  all custom aliases have been checked

### Optimisations

- Rendering;
  https://stackoverflow.com/questions/24120466/writing-http-responses-to-a-temporary-bytes-buffer/24121613#24121613

