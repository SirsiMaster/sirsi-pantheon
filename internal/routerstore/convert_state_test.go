package routerstore

import "testing"

// Convert links are written only by ConvertItemToTask; generic state writes must not forge one.
func TestGenericStateCannotWriteConvertLinks(t *testing.T) {
	s := openTestStore(t)
	key := convertLinkPrefix + "some-item"
	if err := s.SetState(key, `{"agent":"ra","task_id":"x"}`); err == nil {
		t.Fatal("SetState wrote a convert link")
	}
	if ok, err := s.CompareAndSwapState(key, "", false, `{"agent":"ra","task_id":"x"}`); err == nil || ok {
		t.Fatalf("CompareAndSwapState wrote a convert link: ok=%v err=%v", ok, err)
	}
}
