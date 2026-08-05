package rules

import "github.com/SirsiMaster/sirsi-pantheon/internal/jackal"

// ═══════════════════════════════════════════
// AI / ML — Model caches, training artifacts
// ═══════════════════════════════════════════
//
// Every AI cache rule is wrapped in envGuardedRule (where a runtime env var
// pins the cache) and carries minAgeDays=30. Together they ensure the scan
// never reports an actively-used model cache as waste: env-var awareness
// covers the "the user told their runtime to pin this path" case; mtime age
// covers "the user touched it recently." A path that fails neither check IS
// cold cache and is correctly classified safe-to-delete.

const aiCacheMinAgeDays = 30

// NewHuggingFaceCacheRule scans for HuggingFace downloaded models.
// Excluded from results when HF_HOME / HUGGINGFACE_HUB_CACHE / TRANSFORMERS_CACHE
// pin the path, or when its mtime is within the last 30 days.
func NewHuggingFaceCacheRule() jackal.ScanRule {
	return &envGuardedRule{
		baseScanRule: &baseScanRule{
			name:        "huggingface_cache",
			displayName: "HuggingFace Models",
			category:    jackal.CategoryAI,
			description: "Cold HuggingFace Hub model weights (unused 30+ days, no runtime pin)",
			platforms:   []string{"darwin", "linux"},
			// One finding per model repo, not one for the whole hub. The hub
			// directory is an ancestor of every snapshot a local engine serves
			// from, so at hub granularity a single live model suppresses the
			// entire rule — per-repo findings keep the cold models reportable
			// while only the served one drops out.
			paths:      []string{"~/.cache/huggingface/hub/models--*"},
			minAgeDays: aiCacheMinAgeDays,
		},
		envVars: []string{"HF_HOME", "HUGGINGFACE_HUB_CACHE", "TRANSFORMERS_CACHE"},
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
