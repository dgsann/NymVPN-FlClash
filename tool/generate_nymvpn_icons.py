"""Generate platform icon assets from the NymVPN SVG rectangles. Requires Pillow."""
from pathlib import Path
import xml.etree.ElementTree as ET
from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / 'assets_source/images/nymvpn.svg'
rects = ET.parse(SOURCE).getroot().findall('{http://www.w3.org/2000/svg}rect')
def render(size):
    image = Image.new('RGBA', (512, 512))
    draw = ImageDraw.Draw(image)
    for rect in rects:
        x, y, w, h = [int(rect.attrib[key]) for key in ('x', 'y', 'width', 'height')]
        draw.rectangle((x, y, x+w-1, y+h-1), fill=rect.attrib['fill'])
    return image.resize((size, size), Image.Resampling.LANCZOS)

def save(name, size):
    path = ROOT/name
    path.parent.mkdir(parents=True, exist_ok=True)
    image = render(size)
    if path.suffix == '.ico':
        image.save(path, sizes=[(s,s) for s in (16,24,32,48,64,128,256)])
    elif path.suffix == '.webp':
        image.save(path, lossless=True)
    else:
        image.save(path)

save('assets/images/icon.png', 512)
save('assets/images/icon.ico', 256)
save('windows/runner/resources/app_icon.ico', 256)
save('android/app/src/main/ic_launcher-playstore.png', 512)
for density, size in [('mdpi',48),('hdpi',72),('xhdpi',96),('xxhdpi',144),('xxxhdpi',192)]:
    for name in ['ic_launcher', 'ic_launcher_round']:
        save(f'android/app/src/main/res/mipmap-{density}/{name}.webp', size)
    save(f'android/app/src/main/res/mipmap-television-{density}/ic_launcher.webp', size)
for size in [16,32,64,128,256,512,1024]:
    save(f'macos/Runner/Assets.xcassets/AppIcon.appiconset/app_icon_{size}.png', size)

paths = []
for rect in rects[1:]:
    x, y, w, h = [int(rect.attrib[key]) for key in ('x', 'y', 'width', 'height')]
    paths.append(f'M{x},{y}h{w}v{h}h-{w}z')
vector = ('<vector xmlns:android="http://schemas.android.com/apk/res/android" android:width="108dp" android:height="108dp" android:viewportWidth="512" android:viewportHeight="512">'
          + '<path android:fillColor="#BA91FF" android:pathData="' + ' '.join(paths) + '"/></vector>\n')
for name in ['ic_launcher_foreground', 'ic_launcher_foreground_tv']:
    (ROOT/f'android/app/src/main/res/drawable/{name}.xml').write_text(vector, encoding='utf-8')
