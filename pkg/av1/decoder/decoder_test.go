package decoder

import (
	"errors"
	"image"
	"io"
	"path/filepath"
	"testing"

	"github.com/codemodify/av1go-codex/internal/testutil"
	"github.com/codemodify/av1go-codex/pkg/av1"
	"github.com/codemodify/av1go-codex/pkg/av1/obu"
	"github.com/codemodify/av1go-codex/pkg/container/mp4"
)

func TestOpenCodecConfigMain8Bit420(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "main8.mp4",
		Width:    160,
		Height:   90,
		FPS:      5,
		Frames:   5,
		BitDepth: 8,
	})

	dec, err := OpenCodecConfig(codecConfigFromPath(t, fixture.Path))
	if err != nil {
		t.Fatalf("OpenCodecConfig: %v", err)
	}

	md := dec.Metadata()
	if md.Profile != av1.ProfileMain {
		t.Fatalf("Profile = %v", md.Profile)
	}
	if md.BitDepth != 8 {
		t.Fatalf("BitDepth = %d", md.BitDepth)
	}
	if md.Chroma != av1.Chroma420 {
		t.Fatalf("Chroma = %v", md.Chroma)
	}
	if !md.HasSequenceHeader {
		t.Fatalf("expected embedded sequence header")
	}
	if !dec.CanDecodeMain8Bit420() {
		t.Fatalf("expected supported subset")
	}
	if md.Width != fixture.Width || md.Height != fixture.Height {
		t.Fatalf("dimensions = %dx%d, want %dx%d", md.Width, md.Height, fixture.Width, fixture.Height)
	}
}

func TestOpenCodecConfigDetects10BitUnsupportedSubset(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "main10.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   4,
		BitDepth: 10,
	})

	dec, err := OpenCodecConfig(codecConfigFromPath(t, fixture.Path))
	if err != nil {
		t.Fatalf("OpenCodecConfig: %v", err)
	}

	md := dec.Metadata()
	if md.Profile != av1.ProfileMain {
		t.Fatalf("Profile = %v", md.Profile)
	}
	if md.BitDepth != 10 {
		t.Fatalf("BitDepth = %d", md.BitDepth)
	}
	if dec.CanDecodeMain8Bit420() {
		t.Fatalf("expected unsupported subset")
	}
	if !dec.CanDecodeMain10Bit420() {
		t.Fatalf("expected supported main 10-bit 4:2:0 classification")
	}
}

func TestSnapshotParsedInitialCoefQCatUsesPrimaryReferenceQuantizer(t *testing.T) {
	ctx := &obu.FrameContext{}
	ctx.Refs[2] = &obu.FrameHeader{Quantization: obu.Quantization{YAC: 80}}
	hdr := &obu.FrameHeader{
		PrimaryRefFrame: 0,
		Quantization:    obu.Quantization{YAC: 160},
	}
	for i := range hdr.RefIdx {
		hdr.RefIdx[i] = -1
	}
	hdr.RefIdx[0] = 2

	if got, want := snapshotParsedInitialCoefQCat(hdr, ctx), 2; got != want {
		t.Fatalf("initial coefficient qcat = %d, want %d", got, want)
	}
}

func TestDecoderInitialCoefQCatTracksReferenceCDFContext(t *testing.T) {
	dec := &Decoder{}
	key := &obu.FrameHeader{
		PrimaryRefFrame:   7,
		RefreshFrameFlags: 0xff,
		Quantization:      obu.Quantization{YAC: 80},
	}
	keyQCat := dec.snapshotInitialCoefQCat(key)
	if keyQCat != 2 {
		t.Fatalf("key qcat = %d, want 2", keyQCat)
	}
	dec.applyCDFRefRefresh(key, keyQCat)

	p := &obu.FrameHeader{
		PrimaryRefFrame:   0,
		RefreshFrameFlags: 1 << 2,
		Quantization:      obu.Quantization{YAC: 160},
	}
	for i := range p.RefIdx {
		p.RefIdx[i] = -1
	}
	p.RefIdx[0] = 0
	pQCat := dec.snapshotInitialCoefQCat(p)
	if pQCat != 2 {
		t.Fatalf("inter qcat = %d, want inherited 2", pQCat)
	}
	dec.applyCDFRefRefresh(p, pQCat)

	next := &obu.FrameHeader{
		PrimaryRefFrame: 0,
		Quantization:    obu.Quantization{YAC: 206},
	}
	for i := range next.RefIdx {
		next.RefIdx[i] = -1
	}
	next.RefIdx[0] = 2
	if got := dec.snapshotInitialCoefQCat(next); got != 2 {
		t.Fatalf("next qcat = %d, want slot 2 inherited qcat 2", got)
	}
}

func TestDecodeFirstFrameMain8Bit420(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "decode8.mp4",
		Width:    160,
		Height:   90,
		FPS:      5,
		Frames:   5,
		BitDepth: 8,
	})

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	frame, err := dec.NextFrame()
	if err != nil {
		t.Fatalf("NextFrame: %v", err)
	}
	if frame == nil {
		t.Fatal("expected decoded frame")
	}
	if frame.Width != fixture.Width || frame.Height != fixture.Height {
		t.Fatalf("dimensions = %dx%d, want %dx%d", frame.Width, frame.Height, fixture.Width, fixture.Height)
	}
	if frame.BitDepth != 8 {
		t.Fatalf("bit depth = %d, want 8", frame.BitDepth)
	}
	if frame.Layout != av1.Chroma420 {
		t.Fatalf("layout = %v, want 4:2:0", frame.Layout)
	}
	if got, want := frame.Image.Bounds(), image.Rect(0, 0, fixture.Width, fixture.Height); got != want {
		t.Fatalf("image bounds = %v, want %v", got, want)
	}
	if len(frame.Y16) == 0 && (len(frame.Y) == 0 || len(frame.U) == 0 || len(frame.V) == 0) {
		t.Fatal("expected decoded YUV planes")
	}
	if len(frame.U) != 0 && len(frame.V) != 0 && !hasByteNotEqual(frame.U, 128) && !hasByteNotEqual(frame.V, 128) {
		t.Fatal("expected non-neutral decoded chroma")
	}
}

func TestDecodeFirstFrameTenBit(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "decode10.mp4",
		Width:    128,
		Height:   72,
		FPS:      4,
		Frames:   4,
		BitDepth: 10,
	})

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	frame, err := dec.NextFrame()
	if err != nil {
		t.Fatalf("NextFrame: %v", err)
	}
	if frame == nil {
		t.Fatal("expected decoded frame")
	}
	if frame.Width != fixture.Width || frame.Height != fixture.Height {
		t.Fatalf("dimensions = %dx%d, want %dx%d", frame.Width, frame.Height, fixture.Width, fixture.Height)
	}
	if frame.BitDepth != 10 {
		t.Fatalf("bit depth = %d, want 10", frame.BitDepth)
	}
	if len(frame.Y16) == 0 || len(frame.U16) == 0 || len(frame.V16) == 0 {
		t.Fatal("expected decoded YUV planes")
	}
	if !hasUint16NotEqual(frame.U16, 1<<(frame.BitDepth-1)) && !hasUint16NotEqual(frame.V16, 1<<(frame.BitDepth-1)) {
		t.Fatal("expected non-neutral decoded chroma")
	}
}

func TestDecodeShownFrameAfterIntraBCSample(t *testing.T) {
	path := testutil.SamplePath(t, "BigBuckBunny-AV1.mp4")
	testutil.RequireFile(t, path)

	parser, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4(parser): %v", err)
	}
	defer parser.Close()

	targetShown := -1
	shownFrames := 0
	seenIntraBC := false
	for {
		frame, err := parser.NextParsedFrame()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextParsedFrame: %v", err)
		}
		if frame.Header.AllowIntrabc {
			seenIntraBC = true
		}
		if frame.Header.ShowFrame || frame.Header.ShowExistingFrame {
			if seenIntraBC {
				targetShown = shownFrames
				break
			}
			shownFrames++
		}
	}
	if targetShown < 0 {
		t.Skip("sample did not expose a shown frame after intrabc")
	}

	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4(decoder): %v", err)
	}
	defer dec.Close()

	var decoded *Frame
	for i := 0; i <= targetShown; i++ {
		decoded, err = dec.NextFrame()
		if err != nil {
			t.Fatalf("NextFrame(%d): %v", i, err)
		}
	}
	if decoded == nil || decoded.Image == nil {
		t.Fatal("expected decoded image after intrabc frame")
	}
}

func TestDecodeSPBTVToEOF(t *testing.T) {
	path := testutil.SamplePath(t, "spbtv_sample_bipbop_av1_960x540_25fps.mp4")
	testutil.RequireFile(t, path)

	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	shown := 0
	for {
		frame, err := dec.NextFrame()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextFrame(%d): %v", shown, err)
		}
		if frame == nil || frame.Image == nil {
			t.Fatalf("frame %d missing image", shown)
		}
		shown++
	}
	if shown != 375 {
		t.Fatalf("shown frames = %d, want 375", shown)
	}
}

func TestDecodeSPBTVFirstFrameMatchesTileBoundaryPixels(t *testing.T) {
	path := testutil.SamplePath(t, "spbtv_sample_bipbop_av1_960x540_25fps.mp4")
	testutil.RequireFile(t, path)

	dec, err := OpenMP4(path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	frame, err := dec.NextFrame()
	if err != nil {
		t.Fatalf("NextFrame: %v", err)
	}
	if frame == nil || len(frame.Y) == 0 {
		t.Fatal("expected decoded luma plane")
	}

	tests := []struct {
		x, y int
		want byte
	}{
		{x: 512, y: 0, want: 50},
		{x: 512, y: 8, want: 78},
		{x: 512, y: 12, want: 105},
		{x: 640, y: 0, want: 50},
		{x: 640, y: 8, want: 78},
	}
	for _, tc := range tests {
		got := frame.Y[tc.y*frame.YStride+tc.x]
		if got != tc.want {
			t.Fatalf("first-frame Y[%d,%d] = %d, want %d", tc.x, tc.y, got, tc.want)
		}
	}
}

func bundledSmokeFrameBudget(md Metadata) int {
	const (
		maxShownFrames    = 256
		minShownFrames    = 4
		targetSmokePixels = 1280 * 720 * 16
	)
	if md.Width <= 0 || md.Height <= 0 {
		return 32
	}
	perFramePixels := md.Width * md.Height
	if perFramePixels <= 0 {
		return 32
	}
	budget := targetSmokePixels / perFramePixels
	if budget < minShownFrames {
		budget = minShownFrames
	}
	if budget > maxShownFrames {
		budget = maxShownFrames
	}
	return budget
}

func TestBundledSmokeFrameBudgetScalesWithResolution(t *testing.T) {
	tests := []struct {
		name     string
		width    int
		height   int
		expected int
	}{
		{name: "default", width: 0, height: 0, expected: 32},
		{name: "360p", width: 640, height: 360, expected: 64},
		{name: "540p", width: 960, height: 540, expected: 28},
		{name: "1080p", width: 1920, height: 1080, expected: 7},
		{name: "4k", width: 3840, height: 2160, expected: 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := bundledSmokeFrameBudget(Metadata{Width: tc.width, Height: tc.height}); got != tc.expected {
				t.Fatalf("bundledSmokeFrameBudget(%dx%d) = %d, want %d", tc.width, tc.height, got, tc.expected)
			}
		})
	}
}

func TestDecodeBundledSamplesSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bundled sample smoke test in short mode")
	}

	paths := testutil.AvailableSamplePaths(t)
	if len(paths) == 0 {
		t.Skip("no usable local MP4 samples found")
	}

	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			dec, err := OpenMP4(path)
			if err != nil {
				t.Fatalf("OpenMP4: %v", err)
			}
			defer dec.Close()

			maxShownFrames := bundledSmokeFrameBudget(dec.Metadata())
			shown := 0
			for shown < maxShownFrames {
				frame, err := dec.NextFrame()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("NextFrame(%d): %v", shown, err)
				}
				if frame == nil {
					t.Fatalf("frame %d missing decoded frame", shown)
				}
				if frame.Image == nil && len(frame.Y) == 0 && len(frame.Y16) == 0 {
					t.Fatalf("frame %d missing decoded image planes", shown)
				}
				shown++
			}
			if shown == 0 {
				t.Fatal("expected at least one shown frame")
			}
		})
	}
}

func hasByteNotEqual(v []byte, target byte) bool {
	for _, x := range v {
		if x != target {
			return true
		}
	}
	return false
}

func hasUint16NotEqual(v []uint16, target uint16) bool {
	for _, x := range v {
		if x != target {
			return true
		}
	}
	return false
}

func codecConfigFromPath(t *testing.T, path string) []byte {
	t.Helper()

	f, err := mp4.Open(path)
	if err != nil {
		t.Fatalf("mp4.Open: %v", err)
	}
	defer f.Close()

	track, err := f.AV1VideoTrack()
	if err != nil {
		t.Fatalf("AV1VideoTrack: %v", err)
	}
	raw, err := track.AV1C.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	return raw
}

func TestNextParsedFrameMain8Bit420(t *testing.T) {
	fixture := testutil.CreateAV1MP4Fixture(t, t.TempDir(), testutil.AV1FixtureOptions{
		Name:     "parsed.mp4",
		Width:    160,
		Height:   90,
		FPS:      5,
		Frames:   5,
		BitDepth: 8,
	})

	dec, err := OpenMP4(fixture.Path)
	if err != nil {
		t.Fatalf("OpenMP4: %v", err)
	}
	defer dec.Close()

	var (
		parsed     int
		showFrames int
	)
	for {
		frame, err := dec.NextParsedFrame()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextParsedFrame: %v", err)
		}
		if frame.Header.Width != fixture.Width || frame.Header.Height != fixture.Height {
			t.Fatalf("frame dims = %dx%d, want %dx%d", frame.Header.Width, frame.Header.Height, fixture.Width, fixture.Height)
		}
		if !frame.Header.ShowExistingFrame {
			states, err := BuildTileStates(dec.header, &frame.Header, &frame.TileGroup)
			if err != nil {
				t.Fatalf("BuildTileStates: %v", err)
			}
			if len(states) != len(frame.TileGroup.Tiles) {
				t.Fatalf("tile states = %d, want %d", len(states), len(frame.TileGroup.Tiles))
			}
		}
		if parsed == 0 && frame.Header.FrameType != obu.FrameTypeKey {
			t.Fatalf("first parsed frame type = %d, want key", frame.Header.FrameType)
		}
		if !frame.Header.ShowExistingFrame && len(frame.TileGroup.Tiles) == 0 {
			t.Fatal("expected at least one parsed tile")
		}
		if frame.Header.ShowFrame || frame.Header.ShowExistingFrame {
			showFrames++
		}
		parsed++
	}

	if parsed == 0 {
		t.Fatal("expected at least one parsed frame")
	}
	if showFrames == 0 {
		t.Fatal("expected at least one shown frame")
	}
}
