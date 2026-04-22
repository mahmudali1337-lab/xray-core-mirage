package encoding

import (
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"io"
	mrand "math/rand"
	"sync"
	"time"

	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/errors"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	mirageMaxFrame     = 16384
	mirageHeaderLen    = 9
	mirageNoncePrefix  = 8
	mirageNonceLen     = 24
	mirageInitLen      = 32
	mirageMinPad       = 16
	mirageSmallPadCap  = 96
	mirageBigPadCap    = 1280
	mirageBigPadChance = 11
	mirageOverhead     = chacha20poly1305.Overhead
	mirageFrameData    = 0x00
	mirageFrameHeaders = 0x01
	mirageFramePing    = 0x06
)

type MirageSession struct {
	uuid     [16]byte
	isClient bool
	ownRand  [mirageInitLen]byte
	peerRand [mirageInitLen]byte
	ready    chan struct{}
	encAEAD  cipher.AEAD
	decAEAD  cipher.AEAD
	encCtr   uint64
	decCtr   uint64
	streamID uint32
	rngMu    sync.Mutex
	rng      *mrand.Rand
}

func NewMirageSession(uuid [16]byte, isClient bool) *MirageSession {
	s := &MirageSession{
		uuid:     uuid,
		isClient: isClient,
		ready:    make(chan struct{}),
	}
	_, _ = rand.Read(s.ownRand[:])
	var seed int64
	for i := 0; i < 8; i++ {
		seed = (seed << 8) | int64(s.ownRand[i])
	}
	s.rng = mrand.New(mrand.NewSource(seed ^ time.Now().UnixNano()))
	s.streamID = (binary.BigEndian.Uint32(s.ownRand[4:8]) & 0x7fffffff) | 1
	return s
}

func (s *MirageSession) deriveKeys() error {
	var salt [mirageInitLen * 2]byte
	if s.isClient {
		copy(salt[:mirageInitLen], s.ownRand[:])
		copy(salt[mirageInitLen:], s.peerRand[:])
	} else {
		copy(salt[:mirageInitLen], s.peerRand[:])
		copy(salt[mirageInitLen:], s.ownRand[:])
	}
	mk := func(info string) (cipher.AEAD, error) {
		r := hkdf.New(sha256.New, s.uuid[:], salt[:], []byte(info))
		k := make([]byte, chacha20poly1305.KeySize)
		if _, err := io.ReadFull(r, k); err != nil {
			return nil, err
		}
		return chacha20poly1305.NewX(k)
	}
	var err error
	if s.isClient {
		if s.encAEAD, err = mk("MIRAGE/v1/c2s"); err != nil {
			return err
		}
		s.decAEAD, err = mk("MIRAGE/v1/s2c")
	} else {
		if s.encAEAD, err = mk("MIRAGE/v1/s2c"); err != nil {
			return err
		}
		s.decAEAD, err = mk("MIRAGE/v1/c2s")
	}
	return err
}

func (s *MirageSession) randInt(n int) int {
	s.rngMu.Lock()
	defer s.rngMu.Unlock()
	return s.rng.Intn(n)
}

func (s *MirageSession) randBytes(p []byte) {
	s.rngMu.Lock()
	defer s.rngMu.Unlock()
	_, _ = s.rng.Read(p)
}

type MirageWriter struct {
	w        buf.Writer
	sess     *MirageSession
	sentInit bool
	mu       sync.Mutex
}

func NewMirageWriter(w buf.Writer, sess *MirageSession) *MirageWriter {
	return &MirageWriter{w: w, sess: sess}
}

func (w *MirageWriter) writeRaw(p []byte) error {
	b := buf.New()
	if _, err := b.Write(p); err != nil {
		b.Release()
		return err
	}
	return w.w.WriteMultiBuffer(buf.MultiBuffer{b})
}

func (w *MirageWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	defer buf.ReleaseMulti(mb)
	if !w.sentInit {
		if err := w.writeRaw(w.sess.ownRand[:]); err != nil {
			return err
		}
		w.sentInit = true
	}
	<-w.sess.ready
	maxChunk := mirageMaxFrame - mirageOverhead - mirageNoncePrefix - 2 - mirageBigPadCap
	for _, b := range mb {
		if b == nil || b.IsEmpty() {
			continue
		}
		data := b.Bytes()
		for len(data) > 0 {
			n := len(data)
			if n > maxChunk {
				n = maxChunk
			}
			if err := w.writeFrame(mirageFrameData, data[:n]); err != nil {
				return err
			}
			data = data[n:]
		}
	}
	return nil
}

func (w *MirageWriter) writeFrame(frameType byte, payload []byte) error {
	padLen := mirageMinPad + w.sess.randInt(mirageSmallPadCap)
	if w.sess.randInt(100) < mirageBigPadChance {
		padLen = mirageMinPad + w.sess.randInt(mirageBigPadCap)
	}
	pt := make([]byte, 2+len(payload)+padLen)
	binary.BigEndian.PutUint16(pt[:2], uint16(padLen))
	copy(pt[2:], payload)
	w.sess.randBytes(pt[2+len(payload):])

	var nonce [mirageNonceLen]byte
	if _, err := rand.Read(nonce[:mirageNoncePrefix]); err != nil {
		return err
	}
	binary.BigEndian.PutUint64(nonce[mirageNonceLen-8:], w.sess.encCtr)
	w.sess.encCtr++

	ct := w.sess.encAEAD.Seal(nil, nonce[:], pt, nil)
	bodyLen := mirageNoncePrefix + len(ct)
	if bodyLen > mirageMaxFrame {
		return errors.New("mirage: oversized frame")
	}
	frame := make([]byte, mirageHeaderLen+bodyLen)
	frame[0] = byte(bodyLen >> 16)
	frame[1] = byte(bodyLen >> 8)
	frame[2] = byte(bodyLen)
	frame[3] = frameType
	frame[4] = byte(w.sess.randInt(256))
	binary.BigEndian.PutUint32(frame[5:9], w.sess.streamID)
	copy(frame[mirageHeaderLen:mirageHeaderLen+mirageNoncePrefix], nonce[:mirageNoncePrefix])
	copy(frame[mirageHeaderLen+mirageNoncePrefix:], ct)

	out := buf.New()
	if _, err := out.Write(frame); err != nil {
		out.Release()
		return err
	}
	return w.w.WriteMultiBuffer(buf.MultiBuffer{out})
}

type MirageReader struct {
	r       io.Reader
	sess    *MirageSession
	gotInit bool
}

func NewMirageReader(r io.Reader, sess *MirageSession) *MirageReader {
	return &MirageReader{r: r, sess: sess}
}

func (r *MirageReader) ReadMultiBuffer() (buf.MultiBuffer, error) {
	if !r.gotInit {
		if _, err := io.ReadFull(r.r, r.sess.peerRand[:]); err != nil {
			return nil, err
		}
		if err := r.sess.deriveKeys(); err != nil {
			return nil, err
		}
		close(r.sess.ready)
		r.gotInit = true
	}
	for {
		var hdr [mirageHeaderLen]byte
		if _, err := io.ReadFull(r.r, hdr[:]); err != nil {
			return nil, err
		}
		bodyLen := int(hdr[0])<<16 | int(hdr[1])<<8 | int(hdr[2])
		if bodyLen < mirageNoncePrefix+mirageOverhead || bodyLen > mirageMaxFrame {
			return nil, errors.New("mirage: bad frame length")
		}
		body := make([]byte, bodyLen)
		if _, err := io.ReadFull(r.r, body); err != nil {
			return nil, err
		}
		var nonce [mirageNonceLen]byte
		copy(nonce[:mirageNoncePrefix], body[:mirageNoncePrefix])
		binary.BigEndian.PutUint64(nonce[mirageNonceLen-8:], r.sess.decCtr)
		r.sess.decCtr++
		pt, err := r.sess.decAEAD.Open(nil, nonce[:], body[mirageNoncePrefix:], nil)
		if err != nil {
			return nil, errors.New("mirage: decrypt failed").Base(err)
		}
		if len(pt) < 2 {
			return nil, errors.New("mirage: short plaintext")
		}
		padLen := int(binary.BigEndian.Uint16(pt[:2]))
		if 2+padLen > len(pt) {
			return nil, errors.New("mirage: bad padding")
		}
		data := pt[2 : len(pt)-padLen]
		if hdr[3] != mirageFrameData || len(data) == 0 {
			continue
		}
		mb := buf.MultiBuffer{}
		for len(data) > 0 {
			b := buf.New()
			n, _ := b.Write(data)
			mb = append(mb, b)
			data = data[n:]
		}
		return mb, nil
	}
}
