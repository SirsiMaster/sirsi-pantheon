//go:build darwin && cgo

// Package brain — coreml_darwin.go provides the CGO bridge to CoreML on macOS.
// CoreML routes inference to the ANE (Neural Engine) automatically on Apple Silicon.
package brain

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework CoreML
#include "coreml_bridge.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// coremlAvailable returns true if CoreML is functional on this machine.
func coremlAvailable() bool {
	return C.coreml_available() == 1
}

// coremlPredict runs CoreML inference on a compiled model (.mlmodelc).
// Returns the predicted label and confidence score.
func coremlPredict(modelPath, inputPath string) (label string, confidence float64, err error) {
	cModel := C.CString(modelPath)
	cInput := C.CString(inputPath)
	defer C.free(unsafe.Pointer(cModel))
	defer C.free(unsafe.Pointer(cInput))

	result := C.coreml_predict(cModel, cInput)
	defer C.coreml_free_result(&result)

	if result.ok == 0 {
		errMsg := "coreml prediction failed"
		if result.error != nil {
			errMsg = C.GoString(result.error)
		}
		return "", 0, fmt.Errorf("%s", errMsg)
	}

	label = C.GoString(result.label)
	confidence = float64(result.confidence)
	return label, confidence, nil
}
