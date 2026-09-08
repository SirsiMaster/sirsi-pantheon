package main

import (
	"encoding/json"
	"io"
)

type jsonReportEncoder struct{ encoder *json.Encoder }

func newJSONEncoder(w io.Writer) reportEncoder {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return jsonReportEncoder{encoder: encoder}
}

func (e jsonReportEncoder) Encode(value any) error { return e.encoder.Encode(value) }
