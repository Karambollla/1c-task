package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"text/tabwriter"
	"time"

	"edition/internal/edition"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "submit":
		submit(os.Args[2:])
	case "inbox":
		inbox(os.Args[2:])
	case "stats":
		stats(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

type connection struct{ addr, id, secret string }

func common(fs *flag.FlagSet) *connection {
	c := &connection{}
	fs.StringVar(&c.addr, "addr", "127.0.0.1:9000", "server address")
	fs.StringVar(&c.id, "id", "", "client ID")
	fs.StringVar(&c.secret, "secret", "", "client secret")
	return c
}
func (c connection) call(req edition.Request) (edition.Response, error) {
	return edition.Send(c.addr, edition.Request{Type: "hello", ClientID: c.id, Secret: c.secret}, req)
}

func submit(args []string) {
	fs := flag.NewFlagSet("submit", flag.ExitOnError)
	c := common(fs)
	note := fs.String("note-id", "", "idempotency UUID")
	section := fs.String("section", "", "section")
	topic := fs.String("topic", "", "exact topic")
	occurred := fs.String("occurred-at", "", "RFC3339 UTC event time")
	fs.Parse(args)
	if *note == "" {
		*note = newUUID()
	}
	resp, err := c.call(edition.Request{Type: "submit", NoteID: *note, Section: *section, Topic: *topic, OccurredAt: *occurred})
	if err != nil {
		fatal("submit note", "error", err)
	}
	b, _ := json.MarshalIndent(resp.Receipt, "", "  ")
	fmt.Println(string(b))
}

func inbox(args []string) {
	fs := flag.NewFlagSet("inbox", flag.ExitOnError)
	c := common(fs)
	fs.Parse(args)
	resp, err := c.call(edition.Request{Type: "pending_messages"})
	if err != nil {
		fatal("read inbox", "error", err)
	}
	if len(resp.Messages) == 0 {
		fmt.Println("No unread messages.")
		return
	}
	for _, m := range resp.Messages {
		fmt.Printf("[%d] %s\n", m.ID, m.Text)
		if _, err := c.call(edition.Request{Type: "ack_message", MessageID: m.ID}); err != nil {
			slog.Warn("message shown but not acknowledged", "message_id", m.ID, "error", err)
		}
	}
}

func stats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	c := common(fs)
	all := fs.Bool("all", false, "request statistics for all clients (editor only)")
	asJSON := fs.Bool("json", false, "print JSON")
	fs.Parse(args)
	resp, err := c.call(edition.Request{Type: "stats", All: *all})
	if err != nil {
		fatal("request statistics", "error", err)
	}
	if *asJSON {
		b, _ := json.MarshalIndent(resp.Stats, "", "  ")
		fmt.Println(string(b))
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "CLIENT\tACCEPTED\tPRINTED\tSHARE\tSTORIES\tSENSATIONS\tAVG RANK")
	for _, s := range resp.Stats {
		fmt.Fprintf(w, "%s\t%d\t%d\t%.2f%%\t%d\t%d\t%.2f\n", s.ClientID, s.Accepted, s.Printed, s.Share*100, s.Stories, s.Sensations, s.AverageRank)
	}
	w.Flush()
}

func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		fatal("generate UUID", "error", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
func usage() {
	fmt.Fprintln(os.Stderr, "usage: edition-client <submit|inbox|stats>")
	fmt.Fprintln(os.Stderr, "occurred-at example:", time.Now().UTC().Format(time.RFC3339))
}

func fatal(message string, args ...any) {
	slog.Error(message, args...)
	os.Exit(1)
}
