package edition

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"

	"edition/internal/application"
)

type Server struct{ Application *application.Service }

func (s *Server) Serve(listener net.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	dec := json.NewDecoder(io.LimitReader(conn, 8<<20))
	enc := json.NewEncoder(conn)
	var clientID, role string
	for {
		var req Request
		if err := dec.Decode(&req); err != nil {
			return
		}
		ctx := context.Background()
		if clientID == "" {
			if req.Type != "hello" {
				_ = enc.Encode(Response{Type: "error", Error: "hello is required before working messages"})
				return
			}
			var err error
			role, err = s.Application.Authenticate(ctx, req.ClientID, req.Secret)
			if err != nil {
				_ = enc.Encode(Response{Type: "hello", OK: false, Error: err.Error()})
				return
			}
			clientID = req.ClientID
			_ = enc.Encode(Response{Type: "hello", OK: true, Role: role})
			continue
		}
		resp := s.dispatch(ctx, clientID, role, req)
		if err := enc.Encode(resp); err != nil {
			return
		}
	}
}

func (s *Server) dispatch(ctx context.Context, clientID, role string, req Request) Response {
	switch req.Type {
	case "submit":
		if role != RoleReporter {
			return Response{Type: "receipt", Error: "only reporters may submit notes"}
		}
		r, err := s.Application.Submit(ctx, clientID, req.NoteID, req.Section, req.Topic, req.OccurredAt)
		if err != nil {
			return Response{Type: "receipt", Error: err.Error()}
		}
		return Response{Type: "receipt", OK: true, Receipt: &r}
	case "pending_messages":
		items, err := s.Application.Pending(ctx, clientID)
		if err != nil {
			return Response{Type: "messages", Error: err.Error()}
		}
		return Response{Type: "messages", OK: true, Messages: items}
	case "ack_message":
		if err := s.Application.Ack(ctx, clientID, req.MessageID); err != nil {
			return Response{Type: "ack", Error: err.Error()}
		}
		return Response{Type: "ack", OK: true}
	case "stats":
		items, err := s.Application.Stats(ctx, clientID, req.All)
		if err != nil {
			return Response{Type: "stats", Error: err.Error()}
		}
		return Response{Type: "stats", OK: true, Stats: items}
	default:
		return Response{Type: "error", Error: "unknown message type"}
	}
}

func Send(addr string, hello Request, req Request) (Response, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)
	if err = enc.Encode(hello); err != nil {
		return Response{}, err
	}
	var response Response
	if err = dec.Decode(&response); err != nil {
		return Response{}, err
	}
	if !response.OK {
		return response, errors.New(response.Error)
	}
	if err = enc.Encode(req); err != nil {
		return Response{}, err
	}
	if err = dec.Decode(&response); err != nil {
		return Response{}, err
	}
	if !response.OK {
		return response, errors.New(response.Error)
	}
	return response, nil
}
