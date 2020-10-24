package memegen

import "testing"

func BenchmarkMeme(b *testing.B) {
	for n := 0; n < b.N; n++ {
		Impact(1000, 1000,
			"top text here, with wrapping if needed",
			"bottom text too, literal\nnew\nlines are respected")
	}
}
