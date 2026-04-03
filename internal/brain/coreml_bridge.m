// coreml_bridge.m — Objective-C bridge for CoreML inference.
// Provides C-callable functions for Go CGO on darwin.
//
// CoreML on Apple Silicon routes computation to the ANE (Neural Engine)
// automatically when the model supports it. We don't need to specify
// the compute unit — CoreML's compiler decides at load time.

#import <Foundation/Foundation.h>
#import <CoreML/CoreML.h>
#include "coreml_bridge.h"
#include <stdlib.h>
#include <string.h>

// coreml_available checks if the CoreML framework is functional.
// On Apple Silicon, this also implies ANE availability.
int coreml_available(void) {
    // CoreML is available on macOS 10.13+ and all Apple Silicon Macs.
    // We check by attempting to access the MLModel class.
    Class mlModelClass = NSClassFromString(@"MLModel");
    return (mlModelClass != nil) ? 1 : 0;
}

// coreml_predict loads a compiled CoreML model (.mlmodelc) and runs
// prediction using the file at input_path as the feature input.
//
// For file classification, the model takes a file path and outputs
// a class label with confidence. This maps to Brain's FileClass taxonomy.
CoreMLResult coreml_predict(const char *model_path, const char *input_path) {
    CoreMLResult result = {0};

    @autoreleasepool {
        NSError *error = nil;

        // Load compiled model from .mlmodelc directory
        NSString *path = [NSString stringWithUTF8String:model_path];
        NSURL *modelURL = [NSURL fileURLWithPath:path];

        MLModel *model = [MLModel modelWithContentsOfURL:modelURL error:&error];
        if (model == nil) {
            NSString *errMsg = [NSString stringWithFormat:@"model load failed: %@",
                               error.localizedDescription];
            result.ok = 0;
            result.error = strdup([errMsg UTF8String]);
            return result;
        }

        // Create feature provider with the input file path.
        // The model's input schema determines what features are needed.
        // For a file classifier, we pass the file path as a string feature.
        MLModelDescription *desc = model.modelDescription;
        NSDictionary *inputDesc = desc.inputDescriptionsByName;

        // Use the first input feature name from the model schema
        NSString *inputKey = [[inputDesc allKeys] firstObject];
        if (inputKey == nil) {
            result.ok = 0;
            result.error = strdup("model has no input features");
            return result;
        }

        NSString *inputValue = [NSString stringWithUTF8String:input_path];
        MLFeatureValue *featureValue = [MLFeatureValue featureValueWithString:inputValue];

        NSDictionary *features = @{inputKey: featureValue};
        id<MLFeatureProvider> provider =
            [[MLDictionaryFeatureProvider alloc] initWithDictionary:features error:&error];

        if (provider == nil) {
            NSString *errMsg = [NSString stringWithFormat:@"feature provider failed: %@",
                               error.localizedDescription];
            result.ok = 0;
            result.error = strdup([errMsg UTF8String]);
            return result;
        }

        // Run prediction — CoreML routes to ANE/GPU/CPU automatically
        id<MLFeatureProvider> prediction = [model predictionFromFeatures:provider error:&error];
        if (prediction == nil) {
            NSString *errMsg = [NSString stringWithFormat:@"prediction failed: %@",
                               error.localizedDescription];
            result.ok = 0;
            result.error = strdup([errMsg UTF8String]);
            return result;
        }

        // Extract the predicted class label from the first output feature
        NSDictionary *outputDesc = desc.outputDescriptionsByName;
        NSString *outputKey = [[outputDesc allKeys] firstObject];

        if (outputKey != nil) {
            MLFeatureValue *outputValue = [prediction featureValueForName:outputKey];
            if (outputValue.type == MLFeatureTypeString) {
                result.label = strdup([outputValue.stringValue UTF8String]);
                result.confidence = 0.9; // Default high confidence for CoreML
                result.ok = 1;
            } else if (outputValue.type == MLFeatureTypeDictionary) {
                // Classification model — find highest probability class
                NSDictionary *probs = outputValue.dictionaryValue;
                NSString *bestLabel = nil;
                double bestProb = 0.0;
                for (NSString *key in probs) {
                    double prob = [probs[key] doubleValue];
                    if (prob > bestProb) {
                        bestProb = prob;
                        bestLabel = key;
                    }
                }
                if (bestLabel != nil) {
                    result.label = strdup([bestLabel UTF8String]);
                    result.confidence = bestProb;
                    result.ok = 1;
                }
            }
        }

        if (result.ok == 0 && result.error == NULL) {
            result.ok = 0;
            result.error = strdup("no usable output from model");
        }
    }

    return result;
}

void coreml_free_result(CoreMLResult *r) {
    if (r == NULL) return;
    if (r->label != NULL) {
        free((void *)r->label);
        r->label = NULL;
    }
    if (r->error != NULL) {
        free((void *)r->error);
        r->error = NULL;
    }
}
