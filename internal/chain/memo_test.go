package chain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var when = time.Date(2026, 9, 6, 14, 3, 11, 0, time.UTC)

func TestEncodeMemo(t *testing.T) {
	link := Link{N: 347, Act: "I <3 my neighbours & their dog", By: "Olena", CreatedAt: when}
	got, err := EncodeMemo(link, "5abc")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"v":1,"n":347,"act":"I <3 my neighbours & their dog","by":"Olena","prev":"5abc","t":"2026-09-06T14:03:11Z"}`
	if string(got) != want {
		t.Errorf("memo =\n%s\nwant\n%s", got, want)
	}

	first, err := EncodeMemo(Link{N: 0, Act: "the pledge", By: "Nazar", CreatedAt: when}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), `"prev":"genesis"`) {
		t.Errorf("first memo does not point at genesis: %s", first)
	}

	back, err := DecodeMemo(got)
	if err != nil {
		t.Fatal(err)
	}
	if back.N != 347 || back.Act != link.Act || back.By != "Olena" || back.Prev != "5abc" || back.T != "2026-09-06T14:03:11Z" {
		t.Errorf("round trip = %+v", back)
	}
	if _, err := DecodeMemo([]byte(`{"v":2,"n":1}`)); err == nil {
		t.Error("a future memo version was accepted")
	}
}

func TestFitsOnChain(t *testing.T) {
	if err := fitsOnChain(strings.Repeat("a", MaxActLength), strings.Repeat("b", MaxByLength), when); err != nil {
		t.Errorf("the longest plain sentence was refused: %v", err)
	}
	err := fitsOnChain(strings.Repeat("字", MaxActLength), "", when)
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Message != MsgTooBigOnChain {
		t.Errorf("wide text error = %v, want %q", err, MsgTooBigOnChain)
	}
}

func TestGenesisText(t *testing.T) {
	cfg := Config{
		PerLinkCents: 10,
		CapCents:     5000,
		CharityName:  "International Institute of Minnesota",
		CharityURL:   "https://iimn.org",
		PledgerName:  "Nazar",
	}
	want := "I, Nazar, pledge $0.10 for every link added to this chain, up to $50, to the International Institute of Minnesota (iimn.org). Count the links on-chain to hold me to it."
	if got := GenesisText(cfg); got != want {
		t.Errorf("GenesisText =\n%s\nwant\n%s", got, want)
	}
}

func TestMoney(t *testing.T) {
	cases := map[int]string{10: "$0.10", 5: "$0.05", 125: "$1.25", 5000: "$50", 100: "$1"}
	for cents, want := range cases {
		if got := Money(cents); got != want {
			t.Errorf("Money(%d) = %q, want %q", cents, got, want)
		}
	}
}

func TestPledged(t *testing.T) {
	if got := Pledged(3, 10, 5000); got != 30 {
		t.Errorf("Pledged(3) = %d", got)
	}
	if got := Pledged(600, 10, 5000); got != 5000 {
		t.Errorf("Pledged past the cap = %d", got)
	}
}
