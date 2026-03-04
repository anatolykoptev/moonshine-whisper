package main

import (
	"log"
	"sync"
	"time"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

var (
	punctuator *sherpa.OnlinePunctuation
	muPunct    sync.Mutex
)

// initPunctuation loads the CNN-BiLSTM punctuation model if available.
func initPunctuation(modelPath, vocabPath string) {
	punctCfg := &sherpa.OnlinePunctuationConfig{}
	punctCfg.Model.CnnBilstm = modelPath
	punctCfg.Model.BpeVocab = vocabPath
	punctCfg.Model.Provider = "cpu"

	t := time.Now()
	punctuator = sherpa.NewOnlinePunctuation(punctCfg)
	if punctuator == nil {
		log.Printf("WARNING: failed to load punctuation model from %s", modelPath)
		return
	}
	log.Printf("Punctuation model loaded in %.2fs", time.Since(t).Seconds())
}

// addPunctuation adds punctuation to raw transcription text.
// Returns the original text unchanged if punctuator is not loaded.
func addPunctuation(text string) string {
	if punctuator == nil || text == "" {
		return text
	}
	muPunct.Lock()
	defer muPunct.Unlock()
	return punctuator.AddPunct(text)
}
