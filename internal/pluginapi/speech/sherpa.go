package speech

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// The structs of sherpa-onnx's C API (sherpa-onnx/c-api/c-api.h) that the
// app fills, field for field, as of the version it ships (scripts/sherpa).
// A field the app leaves zero is still there: C reads the structs by their
// layout, which Go's matches on the 64-bit systems the app is built for.

type cFeatureConfig struct {
	sampleRate, featureDim int32
}

type cTransducer struct{ encoder, decoder, joiner *byte }
type cOne struct{ model *byte }
type cWhisper struct {
	encoder, decoder, language, task *byte
	tailPaddings                     int32
	tokenTimestamps                  int32
	segmentTimestamps                int32
}
type cCanary struct {
	encoder, decoder, srcLang, tgtLang *byte
	usePnc                             int32
}
type cCohere struct {
	encoder, decoder, language *byte
	usePunct, useITN           int32
}
type cFireRedAsr struct{ encoder, decoder *byte }
type cMoonshine struct{ preprocessor, encoder, uncached, cached, merged *byte }
type cLM struct {
	model *byte
	scale float32
}
type cSenseVoice struct {
	model, language *byte
	useITN          int32
}
type cFunASRNano struct {
	encoderAdaptor, llm, embedding, tokenizer, systemPrompt, userPrompt *byte
	maxNewTokens                                                        int32
	temperature, topP                                                   float32
	seed                                                                int32
	language                                                            *byte
	itn                                                                 int32
	hotwords                                                            *byte
}
type cQwen3ASR struct {
	convFrontend, encoder, decoder, tokenizer *byte
	maxTotalLen, maxNewTokens                 int32
	temperature, topP                         float32
	seed                                      int32
	hotwords                                  *byte
}

type cOfflineModelConfig struct {
	transducer cTransducer
	paraformer cOne
	nemoCTC    cOne
	whisper    cWhisper
	tdnn       cOne

	tokens        *byte
	numThreads    int32
	debug         int32
	provider      *byte
	modelType     *byte
	modelingUnit  *byte
	bpeVocab      *byte
	telespeechCTC *byte

	senseVoice    cSenseVoice
	moonshine     cMoonshine
	fireRedAsr    cFireRedAsr
	dolphin       cOne
	zipformerCTC  cOne
	canary        cCanary
	wenetCTC      cOne
	omnilingual   cOne
	medasr        cOne
	funasrNano    cFunASRNano
	fireRedAsrCTC cOne
	qwen3ASR      cQwen3ASR
	cohere        cCohere
}

type cHomophoneReplacer struct{ dictDir, lexicon, ruleFsts *byte }
type cCtcFstDecoder struct {
	graph     *byte
	maxActive int32
}

type cOfflineRecognizerConfig struct {
	feat           cFeatureConfig
	model          cOfflineModelConfig
	lm             cLM
	decodingMethod *byte
	maxActivePaths int32
	hotwordsFile   *byte
	hotwordsScore  float32
	ruleFsts       *byte
	ruleFars       *byte
	blankPenalty   float32
	hr             cHomophoneReplacer
	ctcFst         cCtcFstDecoder
}

// cOfflineResult is the head of SherpaOnnxOfflineRecognizerResult: the
// app reads its text only.
type cOfflineResult struct {
	text *byte
}

type cSileroVad struct {
	model                            *byte
	threshold, minSilence, minSpeech float32
	windowSize                       int32
	maxSpeech                        float32
}

type cVadModelConfig struct {
	silero     cSileroVad
	sampleRate int32
	numThreads int32
	provider   *byte
	debug      int32
	tenVad     cSileroVad // the same fields as Silero's
}

type cSpeechSegment struct {
	start   int64
	samples *float32
	n       int32
}

// The functions of the C API the app calls.
var sherpa struct {
	createRecognizer  func(*cOfflineRecognizerConfig) uintptr
	destroyRecognizer func(uintptr)
	createStream      func(uintptr) uintptr
	destroyStream     func(uintptr)
	acceptWaveform    func(stream uintptr, rate int32, samples *float32, n int32)
	decode            func(rec, stream uintptr)
	result            func(stream uintptr) *cOfflineResult
	destroyResult     func(*cOfflineResult)

	createVad   func(*cVadModelConfig, float32) uintptr
	destroyVad  func(uintptr)
	vadAccept   func(vad uintptr, samples *float32, n int32)
	vadEmpty    func(uintptr) int32
	vadDetected func(uintptr) int32
	vadFront    func(uintptr) *cSpeechSegment
	vadPop      func(uintptr)
	vadFlush    func(uintptr)
	vadReset    func(uintptr)
	destroySeg  func(*cSpeechSegment)
}

// libNames are sherpa-onnx's C API and the ONNX Runtime it links, as they
// are named on each system. The runtime loads first, from the same
// directory, so that the API finds it there.
func libNames() (api, runtimeLib string) {
	switch runtime.GOOS {
	case "darwin":
		return "libsherpa-onnx-c-api.dylib", "libonnxruntime.dylib"
	case "windows":
		return "sherpa-onnx-c-api.dll", "onnxruntime.dll"
	default:
		return "libsherpa-onnx-c-api.so", "libonnxruntime.so"
	}
}

// ErrUnavailable says there is no speech recognition for this platform.
var ErrUnavailable = errors.New("speech: no sherpa-onnx for this platform")

// libDir returns the directory holding the libraries and the VAD:
// SHERPA_ONNX_DIR, as tests set it, else where Download puts them in root.
func libDir(root string) string {
	if dir := os.Getenv("SHERPA_ONNX_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(root, "sherpa-onnx-"+libVersion)
}

// Available reports whether there is speech recognition for this
// platform, to download.
func Available() bool {
	return len(libFiles[platform()]) > 0 || os.Getenv("SHERPA_ONNX_DIR") != ""
}

func platform() string { return runtime.GOOS + "-" + runtime.GOARCH }

// verifyLibs checks the libraries in dir are those downloaded. They run in
// the app, with what it may reach, as the login in the Keychain: one
// changed since, by whatever could write there, is not loaded.
func verifyLibs(dir string) error {
	for _, f := range natives() {
		if digest(filepath.Join(dir, f.name)) != f.sha256 {
			return fmt.Errorf("speech: %s is not the one downloaded", f.name)
		}
	}
	return nil
}

var loaded struct {
	once sync.Once
	err  error
}

// load loads sherpa-onnx's libraries in dir once.
func load(dir string) error {
	loaded.once.Do(func() { loaded.err = loadLibs(dir) })
	return loaded.err
}

func loadLibs(dir string) error {
	if !Available() {
		return ErrUnavailable
	}
	if os.Getenv("SHERPA_ONNX_DIR") == "" {
		if err := verifyLibs(dir); err != nil {
			return err
		}
	}
	api, rt := libNames()
	if _, err := openLibrary(filepath.Join(dir, rt)); err != nil {
		return fmt.Errorf("speech: loading %s: %w", rt, err)
	}
	h, err := openLibrary(filepath.Join(dir, api))
	if err != nil {
		return fmt.Errorf("speech: loading %s: %w", api, err)
	}
	for name, fn := range map[string]any{
		"SherpaOnnxCreateOfflineRecognizer":             &sherpa.createRecognizer,
		"SherpaOnnxDestroyOfflineRecognizer":            &sherpa.destroyRecognizer,
		"SherpaOnnxCreateOfflineStream":                 &sherpa.createStream,
		"SherpaOnnxDestroyOfflineStream":                &sherpa.destroyStream,
		"SherpaOnnxAcceptWaveformOffline":               &sherpa.acceptWaveform,
		"SherpaOnnxDecodeOfflineStream":                 &sherpa.decode,
		"SherpaOnnxGetOfflineStreamResult":              &sherpa.result,
		"SherpaOnnxDestroyOfflineRecognizerResult":      &sherpa.destroyResult,
		"SherpaOnnxCreateVoiceActivityDetector":         &sherpa.createVad,
		"SherpaOnnxDestroyVoiceActivityDetector":        &sherpa.destroyVad,
		"SherpaOnnxVoiceActivityDetectorAcceptWaveform": &sherpa.vadAccept,
		"SherpaOnnxVoiceActivityDetectorEmpty":          &sherpa.vadEmpty,
		"SherpaOnnxVoiceActivityDetectorDetected":       &sherpa.vadDetected,
		"SherpaOnnxVoiceActivityDetectorFront":          &sherpa.vadFront,
		"SherpaOnnxVoiceActivityDetectorPop":            &sherpa.vadPop,
		"SherpaOnnxVoiceActivityDetectorFlush":          &sherpa.vadFlush,
		"SherpaOnnxVoiceActivityDetectorReset":          &sherpa.vadReset,
		"SherpaOnnxDestroySpeechSegment":                &sherpa.destroySeg,
	} {
		sym, err := librarySymbol(h, name)
		if err != nil || sym == 0 {
			return fmt.Errorf("speech: %s has no %s", api, name)
		}
		purego.RegisterFunc(fn, sym)
	}
	return nil
}

// cstrings holds C strings for a call, alive until it returns.
type cstrings [][]byte

func (cs *cstrings) of(s string) *byte {
	if s == "" {
		return nil
	}
	b := append([]byte(s), 0)
	*cs = append(*cs, b)
	return &b[0]
}

// goString copies the C string at p.
func goString(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}
