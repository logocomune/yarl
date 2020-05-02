# YARL - Yet Another Rate Limit
[![Build Status](https://travis-ci.org/logocomune/yarl.svg?branch=master)](https://travis-ci.org/logocomune/yarl)
[![Go Report Card](https://goreportcard.com/badge/github.com/logocomune/yarl)](https://goreportcard.com/report/github.com/logocomune/yarl)
[![codecov](https://codecov.io/gh/logocomune/yarl/branch/master/graph/badge.svg)](https://codecov.io/gh/logocomune/yarl)
YARL is a golang library that limits the rate of operations per unit time.  

It uses as backend:
 - in memory (lru cache)
 - redis (redigo and radix)
     
## Installation

`go get -u github.com/logocomune/yarl`


