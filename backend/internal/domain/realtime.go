package domain

import "time"

type FrameType string

const (
	FrameAuth            FrameType = "auth"
	FramePing            FrameType = "ping"
	FrameReady           FrameType = "ready"
	FrameNotificationNew FrameType = "notification.new"
	FrameEntityChanged   FrameType = "entity.changed"
	FramePong            FrameType = "pong"
	FrameError           FrameType = "error"
	FrameMessageNew      FrameType = "message.new"
	FrameMessageUpdated  FrameType = "message.updated"
	FrameMessageDeleted  FrameType = "message.deleted"
	FrameReadUpdated     FrameType = "read.updated"
	FrameTypingStart     FrameType = "typing.start"
	FrameTypingStop      FrameType = "typing.stop"
	FrameTyping          FrameType = "typing"
	FramePresence        FrameType = "presence"
)

const PresenceWindow = 90 * time.Second

func ClientFrameTypes() []FrameType {
	return []FrameType{FrameAuth, FramePing, FrameTypingStart, FrameTypingStop}
}

func ServerFrameTypes() []FrameType {
	return []FrameType{
		FrameReady, FrameNotificationNew, FrameEntityChanged, FramePong, FrameError, FrameMessageNew, FrameMessageUpdated, FrameMessageDeleted,
		FrameReadUpdated, FrameTyping, FramePresence,
	}
}

type CloseCode int

const (
	CloseClient          CloseCode = 1000
	CloseShutdown        CloseCode = 1001
	CloseBinaryFrame     CloseCode = 1003
	CloseFrameTooLarge   CloseCode = 1009
	CloseReconnect       CloseCode = 1012
	CloseTryLater        CloseCode = 1013
	CloseUnauthenticated CloseCode = 4401
	CloseTooMany         CloseCode = 4429
)

func CloseCodes() []CloseCode {
	return []CloseCode{
		CloseClient, CloseShutdown, CloseBinaryFrame, CloseFrameTooLarge, CloseReconnect, CloseTryLater,
		CloseUnauthenticated, CloseTooMany,
	}
}

func (c CloseCode) Reason() string {
	switch c {
	case CloseClient:
		return "closed"
	case CloseShutdown:
		return "server shutdown"
	case CloseBinaryFrame:
		return "binary frame"
	case CloseFrameTooLarge:
		return "frame too large"
	case CloseReconnect:
		return "reconnect"
	case CloseTryLater:
		return "try later"
	case CloseUnauthenticated:
		return "unauthenticated"
	case CloseTooMany:
		return "too many"
	}
	return "closed"
}
