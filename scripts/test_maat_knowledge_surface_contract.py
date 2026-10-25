"""Inert source-contract mutations only; no Go, CLI, archive, ingestion or network."""
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent
GUARD = 'scripts/verify-maat-knowledge-surface-contract.sh'
KNOWLEDGE = 'cmd/sirsi/maatknowledge.go'
NATIVE = 'macapp/Sources/SirsiMenubar/MaatCasebookView.swift'
INPUTS = (
    GUARD, 'internal/dashboard/actions.go', KNOWLEDGE,
    'internal/maat/knowledge/knowledge.go', 'internal/mcp/tools.go',
    'internal/mcp/maat_reads.go', 'internal/dashboard/maat.go', NATIVE,
    'contracts/stacklab/maat-system-one-recipe-v1.json',
)


class MaatKnowledgeSurfaceContractTests(unittest.TestCase):
    def check_variant(self, path=None, old=None, replacement=None):
        with tempfile.TemporaryDirectory(prefix='maat-knowledge-contract-') as directory:
            root = Path(directory)
            for name in INPUTS:
                target = root / name
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(ROOT / name, target)
            if path:
                target = root / path
                text = target.read_text()
                self.assertEqual(text.count(old), 1, (path, old))
                target.write_text(text.replace(old, replacement, 1))
            return subprocess.run(['/bin/sh', str(root / GUARD)], text=True,
                                  capture_output=True, timeout=20)

    def refuses(self, path, old, replacement, diagnostic):
        result = self.check_variant(path, old, replacement)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn(diagnostic, result.stderr)

    def test_current_canonical_archive_routes_and_truthful_copy_pass(self):
        result = self.check_variant()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("Ma'at knowledge surface contract: pass", result.stdout)

    def test_refresh_requires_context_bound_canonical_call_and_typed_receipt(self):
        for old in (
            'result, refreshErr := knowledge.RefreshLocal(cmd.Context(), home, source, since, "")',
            'if refreshErr != nil {',
            'if result.State != knowledge.ImportComplete && result.State != knowledge.ImportNoNewItems {',
        ):
            with self.subTest(boundary=old):
                self.refuses(KNOWLEDGE, old, '// removed canonical refresh boundary',
                             "Ma'at public knowledge refresh contract missing:")

        call = 'result, refreshErr := knowledge.RefreshLocal(cmd.Context(), home, source, since, "")'
        render = 'maatKnowledgeRefreshReceipt(time.Since(started), result).Render()'
        canonical = call + '\n\t\t' + render
        # Mutate only the canonical receipt. The profile-refusal Render remains,
        # proving that the guard checks adjacency rather than a global marker.
        self.refuses(KNOWLEDGE, canonical,
                     call + '\n\t\t// removed canonical result receipt',
                     "Ma'at canonical local refresh must render its own adjacent typed result")

    def test_refresh_requires_each_independent_public_flag(self):
        for kind, name in (('String', 'source'), ('String', 'since'),
                           ('String', 'profile'), ('Bool', 'all-profiles')):
            old = f'maatKnowledgeRefreshCmd.Flags().{kind}("{name}",'
            with self.subTest(flag=name):
                self.refuses(KNOWLEDGE, old, old.replace(name, 'not-the-public-flag'),
                             "Ma'at public knowledge refresh contract missing:")

    def test_refresh_does_not_silently_enable_profile_readers(self):
        self.refuses(KNOWLEDGE, 'if profile != "" || allProfiles {',
                     'if false {', "Ma'at public knowledge refresh contract missing:")

    def test_refresh_cannot_delegate_to_legacy_rune_even_with_canonical_markers(self):
        old = 'maatKnowledgeRefreshCmd.Flags().Bool("all-profiles", false, "Refresh Chrome history from every local profile")'
        self.refuses(KNOWLEDGE, old, old + '\n_ = seshatIngestCmd.RunE',
                     "Ma'at public knowledge refresh bypasses its local/no-export owner")

    def test_refresh_cannot_register_export_as_an_ingestion_side_effect(self):
        old = 'maatKnowledgeRefreshCmd.Flags().Bool("all-profiles", false, "Refresh Chrome history from every local profile")'
        self.refuses(KNOWLEDGE, old,
                     old + '\nmaatKnowledgeRefreshCmd.Flags().String("export", "", "not allowed")',
                     "Ma'at public knowledge refresh bypasses its local/no-export owner")

    def test_mcp_requires_both_registered_read_seam_and_canonical_archive_owner(self):
        for path, old, diagnostic in (
            ('internal/mcp/tools.go', 'view, err := readMaatKnowledge(home, query)',
             "Ma'at MCP knowledge contract missing:"),
            ('internal/mcp/maat_reads.go', 'return knowledge.LoadPageContext(ctx, home, r.Query)',
             "Ma'at MCP canonical archive read contract missing:"),
        ):
            with self.subTest(path=path):
                self.refuses(path, old, '// removed canonical read boundary', diagnostic)

    def test_refresh_failure_copy_requires_actual_retained_view_and_unknown_branch(self):
        for old in ('retainedViewAvailable: knowledge != nil',
                    'The archive could not be read; its retained contents are unverified.'):
            with self.subTest(boundary=old):
                self.refuses(NATIVE, old, 'incorrect unconditional cache claim',
                             "native Ma'at knowledge refresh contract missing:")


if __name__ == '__main__':
    unittest.main(verbosity=2)
