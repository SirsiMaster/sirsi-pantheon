import unittest
from pathlib import Path

WORKFLOW = Path(__file__).resolve().parents[1] / '.github/workflows/recover-v163-after-consumed.yml'


class WorkflowShapeTests(unittest.TestCase):
    def test_claim_handoff_steps_are_adjacent_and_gated(self):
        text = WORKFLOW.read_text()
        claim = text.index('      - name: Claim reviewed successor\n')
        handoff = text.index('      - name: Prepare successor publisher handoff\n')
        after_claim = text.index('      - name:', claim + 10)
        self.assertEqual(after_claim, handoff)
        self.assertIn("if: success() && steps.successor_handoff.outcome == 'success'", text)
        self.assertIn('id: successor_handoff', text)
        self.assertIn('--reservation "$PANTHEON_SUCCESSOR_CUSTODY/reservation.json"', text)
        self.assertNotIn('--reservation "$PANTHEON_SUCCESSOR_CUSTODY/reservation.json.staged"', text)
        self.assertIn("if: always() && env.PANTHEON_SUCCESSOR_CUSTODY != ''", text)
        self.assertIn('$RUNNER_TEMP/v163-successor-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}.XXXXXXXX', text)

    def test_source_and_controller_split_is_explicit(self):
        text = WORKFLOW.read_text()
        self.assertIn('ref: a5967913c68c03be435ab009f24df01e908681b3', text)
        self.assertIn('"$GITHUB_WORKSPACE/controller/scripts/build-release-pretoolguard-controller.sh"', text)
        self.assertIn('export PANTHEON_SIGN_CLIENT="$GITHUB_WORKSPACE/controller/tools/signing/sirsi-sign"', text)
        self.assertIn('export PANTHEON_SIGN_CLIENT="$GITHUB_WORKSPACE/source/tools/signing/sirsi-sign"', text)
        self.assertIn('group: pantheon-release-refs/tags/v0.24.163', text)
        self.assertIn('github.run_attempt == 1', text)


if __name__ == '__main__':
    unittest.main()
