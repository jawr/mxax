# ma.ax
easey peasey mail routing

## TODO
The following are needed before beta:

- Figure out inbound security; spamhaus/dbl/spamassain/rspamd/etc
- Failed sends in email should trigger a retry or a bounce back to the sender

## Features
Some ideas

- Aliases should have a settable order, or the ability to define a catchall after 
  all custom aliases have been checked (currently checked based on rule length)
- Browser extension to create a temporary email
