# Local Sample Media

Place local AV1-in-MP4 sample files in this directory when you want to exercise the player or run media-dependent smoke tests.

Files matching `testvideo/*.mp4` are ignored by git. You can either copy media here or create local symlinks to files you already have.

Behavior:

- `go run ./cmd/nativeplayer` scans `testvideo/` for usable `.mp4` files.
- If no usable local sample is available, the player falls back to a generated demo clip.
- Media-dependent tests skip when this directory does not contain usable local samples.

Broken symlinks and samples that resolve into blocked external checkouts are treated as unavailable by the test helpers.
