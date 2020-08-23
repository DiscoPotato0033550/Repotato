package main

import (
	"github.com/VTGare/Eugen/database"
)

var (
	starboardQueue = newQueue()
)

type Queue map[database.MessagePair]chan *StarboardEvent

func newQueue() Queue {
	return make(map[database.MessagePair]chan *StarboardEvent)
}

func (q Queue) Push(pair database.MessagePair, event *StarboardEvent) {
	if ch, ok := q[pair]; ok {
		ch <- event
	} else {
		q[pair] = make(chan *StarboardEvent)
		go func() {
			for e := range q[pair] {
				e.Run()
			}
		}()

		q[pair] <- event
	}
}
