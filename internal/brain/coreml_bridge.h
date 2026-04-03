// coreml_bridge.h — C interface for CoreML inference bridge.
// Called from Go via CGO on darwin/arm64.
#ifndef COREML_BRIDGE_H
#define COREML_BRIDGE_H

#include <stdint.h>

// CoreMLResult holds the output of a CoreML prediction.
typedef struct {
    const char *label;      // predicted class label (caller must free)
    double      confidence; // confidence score 0.0–1.0
    int         ok;         // 1 = success, 0 = failure
    const char *error;      // error message if ok==0 (caller must free)
} CoreMLResult;

// coreml_available returns 1 if CoreML runtime is usable on this device.
int coreml_available(void);

// coreml_predict loads the compiled model at model_path (.mlmodelc directory)
// and runs prediction on the input features. Returns a CoreMLResult.
// The caller is responsible for freeing label and error strings.
CoreMLResult coreml_predict(const char *model_path, const char *input_path);

// coreml_free_result releases strings allocated by coreml_predict.
void coreml_free_result(CoreMLResult *r);

#endif // COREML_BRIDGE_H
