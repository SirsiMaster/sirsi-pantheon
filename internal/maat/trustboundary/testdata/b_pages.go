package fixture

type doc struct{}

func (d *doc) InsertPages(src string) {}

// Class B: one InsertPages per client-supplied attachment, no ceiling.
func attach(d *doc, attachments []string) {
	for _, a := range attachments {
		d.InsertPages(a) // want B
	}
}
