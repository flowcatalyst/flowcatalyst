package ingestion

// Split is how a payload becomes records: None, CSV, JSON or XML.
type Split interface {
	Kind() string
	isSplit()
}

// None makes one record of the payload.
type None struct{}

// CSV makes one record per line; Header drops the first line.
type CSV struct {
	Header bool `json:"header"`
}

// JSON makes one record per element of the array at ArrayPath: $ for a
// top-level array, or $.field.field (fields only; the document is streamed).
type JSON struct {
	ArrayPath string `json:"arrayPath"`
}

// XML makes one record per occurrence of Element.
type XML struct {
	Element string `json:"element"`
}

func (None) Kind() string { return "none" }
func (CSV) Kind() string  { return "csv" }
func (JSON) Kind() string { return "json" }
func (XML) Kind() string  { return "xml" }

func (None) isSplit() {}
func (CSV) isSplit()  {}
func (JSON) isSplit() {}
func (XML) isSplit()  {}

func (s None) MarshalJSON() ([]byte, error) { return marshalTagged(s.Kind(), struct{}{}) }
func (s CSV) MarshalJSON() ([]byte, error)  { type plain CSV; return marshalTagged(s.Kind(), plain(s)) }

func (s JSON) MarshalJSON() ([]byte, error) {
	type plain JSON
	return marshalTagged(s.Kind(), plain(s))
}

func (s XML) MarshalJSON() ([]byte, error) { type plain XML; return marshalTagged(s.Kind(), plain(s)) }

// UnmarshalSplit reads a split object by its kind, strictly.
func UnmarshalSplit(data []byte) (Split, error) {
	return decodeTagged(data, "split", map[string]func([]byte) (Split, error){
		"none": variant[Split, None],
		"csv":  variant[Split, CSV],
		"json": variant[Split, JSON],
		"xml":  variant[Split, XML],
	})
}
