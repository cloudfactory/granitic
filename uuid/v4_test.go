package uuid

import (
	"testing"
)

func TestCompliantDefaultV4(t *testing.T) {

	for i := 0; i < 100; i++ {

		u := V4()

		//fmt.Println(u)

		if !ValidV4(u) {
			t.Errorf("Not a valid UUID %s", u)
		}
	}

}

func BenchmarkCompliantDefaultV4(b *testing.B) {
	for i := 0; i < b.N; i++ {
		V4()
	}
}

func BenchmarkCustomV4(b *testing.B) {

	g := newCrypto128().Generate

	for i := 0; i < b.N; i++ {
		V4Custom(g, StandardEncoder)
	}
}

func TestV4Validation(t *testing.T) {

	valid := []string{
		"5dc6748c-5ae5-4d70-9ac9-e947e5f18b51",
		"86084516-86fc-4620-988b-b9a8e66ef9f9",
	}

	for _, u := range valid {

		if !ValidV4(u) {
			t.Errorf("%s not considered valid V4\n", u)
		}

	}

}
