package decoder

import "errors"

type pixelBackend struct{}

func BackendDescription() string {
	return "built-in pure-Go AV1 decoder (8/10-bit 4:2:0); no external pixel backend configured"
}

func newPixelBackend() (*pixelBackend, error) {
	return nil, errors.New("decoder: pixel backend unavailable")
}

func (b *pixelBackend) Close() error {
	return nil
}

func (b *pixelBackend) sendSample(*Sample) (bool, error) {
	return false, errors.New("decoder: pixel backend unavailable")
}

func (b *pixelBackend) getPicture(int) (*Frame, error) {
	return nil, errors.New("decoder: pixel backend unavailable")
}

func (b *pixelBackend) drainPicture() (*Frame, error) {
	return nil, errors.New("decoder: pixel backend unavailable")
}
