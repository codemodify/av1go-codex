package bitio

import "testing"

func TestReaderBitsAndUE(t *testing.T) {
	r := NewReader([]byte{0b10110010, 0b01000000})

	v, err := r.ReadBits(3)
	if err != nil {
		t.Fatalf("ReadBits: %v", err)
	}
	if v != 0b101 {
		t.Fatalf("ReadBits got %b", v)
	}

	flag, err := r.ReadFlag()
	if err != nil {
		t.Fatalf("ReadFlag: %v", err)
	}
	if !flag {
		t.Fatalf("ReadFlag got false")
	}

	ue, err := r.ReadUE()
	if err != nil {
		t.Fatalf("ReadUE: %v", err)
	}
	if ue != 3 {
		t.Fatalf("ReadUE got %d", ue)
	}
}

func TestULEB128(t *testing.T) {
	v, n, err := ReadULEB128([]byte{0xE5, 0x8E, 0x26})
	if err != nil {
		t.Fatalf("ReadULEB128: %v", err)
	}
	if v != 624485 || n != 3 {
		t.Fatalf("ReadULEB128 got value=%d bytes=%d", v, n)
	}
}

func TestReaderHelpers(t *testing.T) {
	r := NewReader([]byte{
		0b00101110, // uniform(10): 001 -> 1, vlc: 0 1 0 -> 1, byte align pads the rest
		0xE5, 0x8E, 0x26,
	})

	uniform, err := r.ReadUniform(10)
	if err != nil {
		t.Fatalf("ReadUniform: %v", err)
	}
	if uniform != 1 {
		t.Fatalf("ReadUniform got %d", uniform)
	}

	vlc, err := r.ReadVLC()
	if err != nil {
		t.Fatalf("ReadVLC: %v", err)
	}
	if vlc != 2 {
		t.Fatalf("ReadVLC got %d", vlc)
	}

	r.ByteAlign()
	if got, want := r.Position(), 8; got != want {
		t.Fatalf("Position after ByteAlign = %d, want %d", got, want)
	}

	uleb, err := r.ReadULEB128()
	if err != nil {
		t.Fatalf("ReadULEB128: %v", err)
	}
	if uleb != 624485 {
		t.Fatalf("ReadULEB128 got %d", uleb)
	}
}

func TestReadSBitsAndSubexp(t *testing.T) {
	r := NewReader([]byte{
		0b11110000, // signed 4 bits = -1, subexp first bucket bits = 000
	})

	signed, err := r.ReadSBits(4)
	if err != nil {
		t.Fatalf("ReadSBits: %v", err)
	}
	if signed != -1 {
		t.Fatalf("ReadSBits got %d", signed)
	}

	v, err := r.ReadSubexp(0, 2)
	if err != nil {
		t.Fatalf("ReadSubexp: %v", err)
	}
	if v != 0 {
		t.Fatalf("ReadSubexp got %d", v)
	}
}
