# Iznik Server Go

Iznik is a platform for online reuse of unwanted items.  This is a work-in-progress 
implementation of a server in Go.  

## What this is for
The aim is to provide fast read-only access, so that we can:
* Render pages significantly faster than when using the [PHP server](https://github.com/Freegle/iznik-server).  Language wars are dull, but Go is faster, and the easy parallelisation which goroutines offer make it possible to reduce the latency of individual calls dramatically.
* Reduce the CPU load on our limited server hardware.  Most Freegle workload is read operations, and Go is much lighter on CPU than PHP.

So although having two servers in different languages is an niffy architecture smell, the nifty practical benefits are huge. 

## What this is not for

These are out of scope:
* Access to data which is private to moderators.
* Write-access or any kind of actions.  Did I mention this is purely aimed at the read case?

Those things are done using the PHP API.  For that reason the CircleCI testing for this server installs the PHP server code to set up a test environment.
  
## Status

**This is a WIP.**

What works:
* Groups
* Messages
* Users
* Addresses

What doesn't work:
* Forcing login at appropriate points.
* Chat (getting there)
* Settings
* ChitChat
* Stories
* Volunteer Ops
* Stats

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