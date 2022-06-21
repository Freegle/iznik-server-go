# Iznik Server Go

Iznik is a platform for online reuse of unwanted items.  This is a work-in-progress 
implementation of the server in Go.  The initial aim is to provide fast read-only access, 
so that we can render pages significantly faster than the [PHP server](https://github.com/Freegle/iznik-server).

## Status

This is a WIP.

What works - read-only access to:
* Groups
* Messages
* Users (including the logged in user via a JWT)

What doesn't work:
* Chat
* Settings
* ChitChat
* Stories
* Volunteer Ops
* Stats

What this server is not for:
* Access to data which is private to moderators.
* Write-access or any kind of actions.
...which are done using the older PHP API.  For that reason the CircleCI testing for this server uses the PHP server to set up a test environment.

## Funding
The development has been funded by Freegle for use in the UK,
but it is an open source platform which can be used or adapted by others.

**It would be very lovely if you sponsored us.**

[:heart: Sponsor](https://github.com/sponsors/Freegle)


## License

This code is licensed under the GPL v2 (see LICENSE file).  If you intend to use it, Freegle would be interested to
hear about it; you can mail [geeks@ilovefreegle.org](mailto:geeks@ilovefreegle.org).

[![CircleCI](https://dl.circleci.com/status-badge/img/gh/Freegle/iznik-server-go/tree/master.svg?style=svg)](https://dl.circleci.com/status-badge/redirect/gh/Freegle/iznik-server-go/tree/master)

[![Coverage Status](https://coveralls.io/repos/github/Freegle/iznik-server-go/badge.svg)](https://coveralls.io/github/Freegle/iznik-server-go)