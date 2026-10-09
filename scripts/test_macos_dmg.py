import importlib.util
from pathlib import Path
import subprocess
import sys
import tempfile
import types
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('packager', Path(__file__).with_name('package-macos-dmg.py'))
packager = importlib.util.module_from_spec(spec)
spec.loader.exec_module(packager)


class PackagingTests(unittest.TestCase):
    def test_signature_failure_never_builds_image(self):
        with tempfile.TemporaryDirectory() as temp:
            app = Path(temp).resolve() / 'Space Name.app'
            (app / 'Contents').mkdir(parents=True)
            (app / 'Contents/Info.plist').write_text('fixture')
            calls = []
            fake = types.SimpleNamespace(build_dmg=lambda *a, **kw: calls.append(a))
            with patch.dict(sys.modules, dmgbuild=fake), patch.object(sys, 'argv', [
                'package', str(app), 'Volume', str(Path(temp) / 'out.dmg')
            ]), patch.object(packager.subprocess, 'run', side_effect=subprocess.CalledProcessError(1, 'codesign')):
                with self.assertRaises(subprocess.CalledProcessError):
                    packager.main()
            self.assertEqual(calls, [])

    def test_layout_and_background_available_during_build(self):
        with tempfile.TemporaryDirectory() as temp:
            app = Path(temp).resolve() / 'Space Name.app'
            (app / 'Contents').mkdir(parents=True)
            (app / 'Contents/Info.plist').write_text('fixture')
            calls = []
            def build(output, volume, settings):
                calls.append(output)
                self.assertEqual(volume, 'Volume')
                self.assertEqual(settings['files'], [str(app)])
                self.assertEqual(settings['symlinks'], {'Applications': '/Applications'})
                self.assertLess(settings['icon_locations'][app.name][0], settings['icon_locations']['Applications'][0])
                self.assertTrue(Path(settings['background']).read_bytes().startswith(b'\x89PNG'))
                self.assertEqual(settings['default_view'], 'icon-view')
                self.assertNotIn('hide_extensions', settings)
                self.assertLess(settings['grid_spacing'], 100)
            with patch.dict(sys.modules, dmgbuild=types.SimpleNamespace(build_dmg=build)), patch.object(sys, 'argv', [
                'package', str(app), 'Volume', str(Path(temp) / 'out.dmg')
            ]), patch.object(packager.subprocess, 'run') as verify:
                packager.main()
                verify.assert_called_once_with(['codesign', '--verify', '--deep', '--strict', str(app)], check=True)
            self.assertEqual(len(calls), 1)


if __name__ == '__main__':
    unittest.main()
