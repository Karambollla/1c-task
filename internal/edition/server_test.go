package edition

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"edition/internal/application"
)

func TestTCPHandshakeAndRolePermissions(t *testing.T) {
	s, _ := testStore(t, 1.05)
	addReporter(t, s, "reporter", .5)
	if err := s.CreateUser(context.Background(), "editor", RoleEditor, "editor-secret", 0); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() { _ = (&Server{Application: application.New(s)}).Serve(ln) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(conn).Encode(Request{Type: "stats"}); err != nil {
		t.Fatal(err)
	}
	var denied Response
	if err := json.NewDecoder(conn).Decode(&denied); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if denied.Error == "" {
		t.Fatalf("working message before hello was accepted: %#v", denied)
	}

	_, err = Send(ln.Addr().String(), Request{Type: "hello", ClientID: "reporter", Secret: "reporter-secret"}, Request{Type: "stats", All: true})
	if err == nil {
		t.Fatal("reporter received all statistics")
	}
	response, err := Send(ln.Addr().String(), Request{Type: "hello", ClientID: "editor", Secret: "editor-secret"}, Request{Type: "stats", All: true})
	if err != nil || len(response.Stats) != 2 {
		t.Fatalf("editor stats = %#v, %v", response, err)
	}
}
