// Package mailops implements Sirsi mail hygiene: detecting inbox messages
// whose empty text/plain MIME part hangs Outlook's IMAP sync loop, and
// archiving them (never deleting, never emptying trash, never sending).
// Governed by 𓆄 Ma'at (quality/safety gate) under the Anubis hygiene domain
// (docs/DEITY_REGISTRY.md). See docs/design-notes/MAILOPS_DESIGN.md.
package mailops

// Message is the subset of a Gmail message this package reasons about.
type Message struct {
	ID      string
	Date    string
	From    string
	Subject string
}

// Client is the mailbox side-effect boundary (Rule A16 injection pattern).
// PoisonScan/SenderCensus depend on this interface, never on the Gmail SDK
// directly, so both are testable without network access.
type Client interface {
	// ListInboxIDs returns inbox message IDs matching the given Gmail query.
	ListInboxIDs(query string) ([]string, error)
	// HasEmptyTextPart reports whether any of the given message IDs have a
	// multipart body whose text/plain part is zero bytes (the shape that
	// hangs Outlook's IMAP sync). Returns only the IDs that match.
	HasEmptyTextPart(ids []string) ([]string, error)
	// Headers fetches From/Subject/Date for the given message IDs.
	Headers(ids []string) (map[string]Message, error)
	// SendersLastYear returns, per From address, the message count and
	// whether any message from that sender carried List-Unsubscribe.
	SendersLastYear() (counts map[string]int, hasListUnsubscribe map[string]bool, err error)
	// Archive removes the INBOX label from the given message IDs. Never
	// deletes, never trashes, never sends. The only mutating call in Client.
	Archive(ids []string) error
}

// PoisonResult is the outcome of a poison scan.
type PoisonResult struct {
	ScannedTotal int
	Poisoned     []Message
	Archived     []string // set only when Apply was true
}

// PoisonScan finds inbox messages with an empty text part. When apply is
// true, matched messages are archived (INBOX label removed) and the archived
// IDs are recorded in the result and, via log, in a signed receipt.
func PoisonScan(c Client, apply bool, log *ReceiptLog) (*PoisonResult, error) {
	cand, err := c.ListInboxIDs("in:inbox")
	if err != nil {
		return nil, err
	}
	bad, err := c.HasEmptyTextPart(cand)
	if err != nil {
		return nil, err
	}
	res := &PoisonResult{ScannedTotal: len(cand)}
	if len(bad) == 0 {
		return res, nil
	}
	headers, err := c.Headers(bad)
	if err != nil {
		return nil, err
	}
	for _, id := range bad {
		res.Poisoned = append(res.Poisoned, headers[id])
	}
	if !apply {
		return res, nil
	}
	if err := c.Archive(bad); err != nil {
		return nil, err
	}
	res.Archived = bad
	if log != nil {
		if err := log.RecordArchive("poison", bad); err != nil {
			return res, err
		}
	}
	return res, nil
}

// SenderCount is one row of a sender census, sorted by Count descending.
type SenderCount struct {
	From            string
	Count           int
	ListUnsubscribe bool
}

// SenderCensus returns the top n senders to the inbox over the last year.
func SenderCensus(c Client, n int) ([]SenderCount, error) {
	counts, lists, err := c.SendersLastYear()
	if err != nil {
		return nil, err
	}
	out := make([]SenderCount, 0, len(counts))
	for from, k := range counts {
		out = append(out, SenderCount{From: from, Count: k, ListUnsubscribe: lists[from]})
	}
	sortSendersDesc(out)
	if n > 0 && n < len(out) {
		out = out[:n]
	}
	return out, nil
}

func sortSendersDesc(s []SenderCount) {
	// ponytail: insertion sort is fine, sender lists top out in the low
	// thousands; switch to sort.Slice if that ever changes.
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].Count > s[j-1].Count; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
