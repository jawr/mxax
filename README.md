# ma.ax
easey peasey mail routing


## TODO
The following are needed before beta:

- [ ] Rework Return-Path to be: bounce+uuid+original=email.com@domain.com
- [ ] Finish refactoring out SQL from internal/site handlers
- [ ] Outbound SMTP & SMTP Security / Auth
- [ ] Improve log detail view
- [ ] Figure out inbound security; spamhaus/dbl/spamassain/rspamd/etc
- [ ] Graphs based on: accountID, domainID, aliasID, destinationID

## Features
Some ideas

- Aliases should have a settable order, or the ability to define a catchall after 
  all custom aliases have been checked (currently checked based on rule length)
- Browser extension to create a temporary email



## SMTP
Two types, port 25 for forwarding/relaying. 587 for submission.

Submission:
- Authenticate user
- Read RCPT From
- Match domain
- Read RCPT To
- Read DATA
- Add Return-Path?
- Sign
- Queue

Relay
- Read RCPT From
- Read RCPT To
- Match domain
- Match alias
- Read DATA
- Add Received
- Add Return-Path
- Sign
- Get destinations
- Queue
