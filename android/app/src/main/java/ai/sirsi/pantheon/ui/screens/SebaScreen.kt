package ai.sirsi.pantheon.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import ai.sirsi.pantheon.R
import ai.sirsi.pantheon.bridge.PantheonBridge
import ai.sirsi.pantheon.models.AcceleratorProfile
import ai.sirsi.pantheon.models.HardwareProfile
import ai.sirsi.pantheon.ui.components.DeityHeader
import ai.sirsi.pantheon.ui.theme.PantheonError
import ai.sirsi.pantheon.ui.theme.PantheonGold
import ai.sirsi.pantheon.ui.theme.PantheonSurface

/**
 * Seba hardware detection screen. Probes device hardware capabilities
 * and available compute accelerators.
 */
@Composable
fun SebaScreen() {
    var isDetecting by remember { mutableStateOf(false) }
    var hardwareProfile by remember { mutableStateOf<HardwareProfile?>(null) }
    var acceleratorProfile by remember { mutableStateOf<AcceleratorProfile?>(null) }
    var errorMessage by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 24.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        DeityHeader(
            glyph = "\uD80C\uDFBC",
            name = stringResource(R.string.seba_name),
            subtitle = stringResource(R.string.seba_subtitle),
            description = stringResource(R.string.seba_description),
        )

        Button(
            onClick = {
                scope.launch {
                    isDetecting = true
                    errorMessage = null
                    try {
                        hardwareProfile = PantheonBridge.sebaDetectHardware()
                        acceleratorProfile = PantheonBridge.sebaDetectAccelerators()
                    } catch (e: Exception) {
                        errorMessage = e.message ?: stringResource(R.string.error_generic)
                    } finally {
                        isDetecting = false
                    }
                }
            },
            enabled = !isDetecting,
            modifier = Modifier.fillMaxWidth(),
            colors = ButtonDefaults.buttonColors(containerColor = PantheonGold),
            shape = RoundedCornerShape(8.dp),
        ) {
            if (isDetecting) {
                CircularProgressIndicator(
                    color = MaterialTheme.colorScheme.onPrimary,
                    strokeWidth = 2.dp,
                )
            } else {
                Text(stringResource(R.string.action_detect))
            }
        }

        // Error
        if (errorMessage != null) {
            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(containerColor = PantheonError.copy(alpha = 0.1f)),
                shape = RoundedCornerShape(8.dp),
            ) {
                Text(
                    text = errorMessage!!,
                    modifier = Modifier.padding(16.dp),
                    color = PantheonError,
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }

        // Hardware Profile
        if (hardwareProfile != null) {
            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(containerColor = PantheonSurface),
                shape = RoundedCornerShape(12.dp),
            ) {
                Column(modifier = Modifier.padding(16.dp)) {
                    Text(
                        text = "Device Hardware",
                        style = MaterialTheme.typography.titleMedium,
                        color = PantheonGold,
                    )
                    Spacer(modifier = Modifier.height(8.dp))
                    InfoRow(stringResource(R.string.label_cpu), hardwareProfile!!.cpuModel)
                    InfoRow(stringResource(R.string.label_arch), hardwareProfile!!.cpuArch)
                    InfoRow(stringResource(R.string.label_cores), "${hardwareProfile!!.cpuCores}")
                    InfoRow(stringResource(R.string.label_ram), hardwareProfile!!.formattedRAM)
                    hardwareProfile!!.gpu?.let { gpu ->
                        InfoRow(stringResource(R.string.label_gpu), gpu.name)
                    }
                    hardwareProfile!!.os?.let { os ->
                        InfoRow("OS", os)
                    }
                }
            }
        }

        // Accelerator Profile
        if (acceleratorProfile != null) {
            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(containerColor = PantheonSurface),
                shape = RoundedCornerShape(12.dp),
            ) {
                Column(modifier = Modifier.padding(16.dp)) {
                    Text(
                        text = "Compute Accelerators",
                        style = MaterialTheme.typography.titleMedium,
                        color = PantheonGold,
                    )
                    Spacer(modifier = Modifier.height(8.dp))
                    InfoRow("GPU", if (acceleratorProfile!!.hasGpu) "Available" else "None")
                    acceleratorProfile!!.gpuVendor?.let { InfoRow("GPU Vendor", it) }
                    acceleratorProfile!!.gpuCores?.let { InfoRow("GPU Cores", "$it") }
                    InfoRow("CPU Cores", "${acceleratorProfile!!.cpuCores}")
                    InfoRow("CUDA", if (acceleratorProfile!!.hasCuda) "Yes" else "No")
                    InfoRow("ROCm", if (acceleratorProfile!!.hasRocm) "Yes" else "No")
                    acceleratorProfile!!.routing?.let { InfoRow("Routing", it) }
                }
            }
        }
    }
}
