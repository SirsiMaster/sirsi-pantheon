package rules

import "github.com/SirsiMaster/sirsi-pantheon/internal/jackal"

// ═══════════════════════════════════════════
// AI / ML — Model caches, training artifacts
// ═══════════════════════════════════════════
//
// Every AI cache rule is wrapped in envGuardedRule (where a runtime env var
// pins the cache) and carries minAgeDays=30. The HuggingFace rule additionally
// reads ~/.sirsi/gemma-model.conf to protect the configured Sirsi inference
// substrate even when its cache mtime is cold. A path that passes all guards IS
// cold cache and is correctly classified safe-to-delete.

const aiCacheMinAgeDays = 30

// NewHuggingFaceCacheRule scans for HuggingFace downloaded models.
// Excluded from results when:
//   - HF_HOME / HUGGINGFACE_HUB_CACHE / TRANSFORMERS_CACHE pin the path, OR
//   - the directory mtime is within the last 30 days, OR
//   - the model ID appears in ~/.sirsi/gemma-model.conf or gemma-model-max.conf
//     (the Sirsi inference substrate — never reclaimable, even if cold).
func NewHuggingFaceCacheRule() jackal.ScanRule {
	return &envGuardedRule{
		baseScanRule: &baseScanRule{
			name:        "huggingface_cache",
			displayName: "HuggingFace Models",
			category:    jackal.CategoryAI,
			description: "Cold HuggingFace Hub model weights (unused 30+ days, no runtime pin, not the configured Sirsi model)",
			platforms:   []string{"darwin", "linux"},
			paths:       []string{"~/.cache/huggingface/hub"},
			minAgeDays:  aiCacheMinAgeDays,
		},
		envVars:     []string{"HF_HOME", "HUGGINGFACE_HUB_CACHE", "TRANSFORMERS_CACHE"},
		livePathFns: []func(string) []string{sirsiGemmaLivePaths},
	}
}

// NewOllamaModelsRule scans for Ollama local models.
func NewOllamaModelsRule() jackal.ScanRule {
	return &envGuardedRule{
		baseScanRule: &baseScanRule{
			name:        "ollama_models",
			displayName: "Ollama Models",
			category:    jackal.CategoryAI,
			description: "Cold Ollama model weights (unused 30+ days, no runtime pin)",
			platforms:   []string{"darwin", "linux"},
			paths:       []string{"~/.ollama/models"},
			minAgeDays:  aiCacheMinAgeDays,
		},
		envVars: []string{"OLLAMA_MODELS"},
	}
}

// NewPyTorchCacheRule scans for PyTorch hub and model cache.
func NewPyTorchCacheRule() jackal.ScanRule {
	return &envGuardedRule{
		baseScanRule: &baseScanRule{
			name:        "pytorch_cache",
			displayName: "PyTorch Cache",
			category:    jackal.CategoryAI,
			description: "Cold PyTorch hub models and compiled extensions (unused 30+ days)",
			platforms:   []string{"darwin", "linux"},
			paths:       []string{"~/.cache/torch"},
			minAgeDays:  aiCacheMinAgeDays,
		},
		envVars: []string{"TORCH_HOME", "PYTORCH_HOME"},
	}
}

// NewMLXCacheRule scans for Apple MLX model cache.
func NewMLXCacheRule() jackal.ScanRule {
	return &envGuardedRule{
		baseScanRule: &baseScanRule{
			name:        "mlx_cache",
			displayName: "Apple MLX Cache",
			category:    jackal.CategoryAI,
			description: "Cold Apple MLX converted models (unused 30+ days, no runtime pin)",
			platforms:   []string{"darwin"},
			paths:       []string{"~/.cache/mlx", "~/Library/Caches/mlx"},
			minAgeDays:  aiCacheMinAgeDays,
		},
		envVars: []string{"MLX_MODEL_PATH", "MLX_HOME"},
	}
}

// NewMetalShaderCacheRule scans for compiled Metal shader caches.
// Mtime guard only — no canonical env var pins this path.
func NewMetalShaderCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "metal_shaders",
		displayName: "Metal Shader Cache",
		category:    jackal.CategoryAI,
		description: "Cold Metal GPU shader caches (untouched 30+ days)",
		platforms:   []string{"darwin"},
		paths:       []string{"~/Library/Caches/com.apple.metal"},
		minAgeDays:  aiCacheMinAgeDays,
	}
}

// NewTensorFlowCacheRule scans for TensorFlow cache.
func NewTensorFlowCacheRule() jackal.ScanRule {
	return &envGuardedRule{
		baseScanRule: &baseScanRule{
			name:        "tensorflow_cache",
			displayName: "TensorFlow Cache",
			category:    jackal.CategoryAI,
			description: "Cold TensorFlow / Keras model cache (unused 30+ days, no runtime pin)",
			platforms:   []string{"darwin", "linux"},
			paths:       []string{"~/.cache/tensorflow", "~/.keras/models"},
			minAgeDays:  aiCacheMinAgeDays,
		},
		envVars: []string{"TF_HOME", "KERAS_HOME"},
	}
}
