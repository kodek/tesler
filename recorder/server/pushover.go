package main

import "github.com/gregdel/pushover"

// PushoverFacade is a wrapper on a Pushover client and a recipient. It simplifies function signatures that depend on both.
type PushoverFacade struct {
	push      *pushover.Pushover
	recipient *pushover.Recipient
}

func (p *PushoverFacade) SendMessageWithTitle(message, title string) (*pushover.Response, error) {
	return p.push.SendMessage(pushover.NewMessageWithTitle(message, title), p.recipient)
}
