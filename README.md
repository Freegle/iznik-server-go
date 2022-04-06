# Iznik Server Go

Iznik is a platform for online reuse of unwanted items.  This is a work-in-progress 
implementation of the server in Go.  The initial aim is to provide fast read-only access, 
so that we can render pages more rapidly.

## Status

So far this a proof of concept to see whether we can have a Go
version of the server which is significantly faster than the [PHP version](https://github.com/Freegle/iznik-server).

What works:
* /api/message/:id
* /api/group/:id

What doesn't work:
* Access to data which is private to a logged-in user or to mods.
* Write-access or any kind of actions.
* Everything else.

## Funding
The development has been funded by Freegle for use in the UK,
but it is an open source platform which can be used or adapted by others.

**It would be very lovely if you sponsored us.**

[:heart: Sponsor](https://github.com/sponsors/Freegle)


## License

This code is licensed under the GPL v2 (see LICENSE file).  If you intend to use it, Freegle would be interested to
hear about it; you can mail [geeks@ilovefreegle.org](mailto:geeks@ilovefreegle.org).