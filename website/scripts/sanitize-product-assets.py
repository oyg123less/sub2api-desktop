from __future__ import annotations

import argparse
import hashlib
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter, ImageFont


ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / "public" / "screenshots" / "v044"

FONT_REGULAR = Path("C:/Windows/Fonts/msyh.ttc")
FONT_BOLD = Path("C:/Windows/Fonts/msyhbd.ttc")

WHITE = (255, 255, 255)
INK = (39, 39, 35)
MUTED = (155, 149, 139)
ORANGE = (207, 91, 55)
INPUT = (247, 244, 238)
SHARING_INPUT = (241, 238, 229)
PAGE = (248, 247, 242)

SOURCE_HASHES = {
    "dashboard": "ae975e444ef559f3b0bc33835383b797a0e291571d974a31ba7995f2e6236c72",
    "accounts": "4ee4e1bc2cc9f1b7721ac267cd445e357b2c86075fb24876f953c855354debda",
    "network": "6160a23e9cd6baa0db41caf5bf8085ae808d9967120e8f90b24f8d530e6af092",
    "sharing": "6a86ee1c59d29301a91a921db71e2c015a20b1e7ffa8ee89fea8524b1ea2fc71",
    "codex": "7834cfa8402b7ad9570baea94e150520ae79e8d5715e6aa853926c72b1a53a16",
}


def font(size: int, *, bold: bool = False) -> ImageFont.FreeTypeFont:
    path = FONT_BOLD if bold else FONT_REGULAR
    return ImageFont.truetype(str(path), size=size)


def require_image(path: Path, expected_sha256: str) -> Image.Image:
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    if digest != expected_sha256:
        raise ValueError(f"Source screenshot hash mismatch; review redaction coordinates before using: {path}")
    image = Image.open(path).convert("RGB")
    if image.size != (1924, 1277):
        raise ValueError(f"Expected a 1924x1277 screenshot, got {image.size}: {path}")
    return image


def replace_text(
    draw: ImageDraw.ImageDraw,
    box: tuple[int, int, int, int],
    text: str,
    *,
    fill: tuple[int, int, int] = WHITE,
    color: tuple[int, int, int] = INK,
    size: int = 20,
    bold: bool = False,
    inset: tuple[int, int] = (4, 2),
) -> None:
    draw.rectangle(box, fill=fill)
    draw.text(
        (box[0] + inset[0], box[1] + inset[1]),
        text,
        fill=color,
        font=font(size, bold=bold),
    )


def sanitize_dashboard(image: Image.Image) -> Image.Image:
    image = image.copy()
    draw = ImageDraw.Draw(image)

    metrics = [
        ((394, 530, 700, 590), "6"),
        ((773, 530, 1075, 590), "128"),
        ((1152, 530, 1460, 590), "12.8M"),
        ((1530, 530, 1838, 590), "$24.6000"),
    ]
    for box, value in metrics:
        replace_text(draw, box, value, size=39, bold=True, inset=(5, 1))

    replace_text(
        draw,
        (1145, 830, 1735, 875),
        "sk-local********demo",
        fill=INPUT,
        color=ORANGE,
        size=22,
        inset=(4, 2),
    )

    recent_rows = [
        ((1460, 1097, 1831, 1138), "18.4K tok    1.2s    12:04:18"),
        ((1460, 1170, 1831, 1211), "21.7K tok    1.4s    11:58:42"),
        ((1460, 1242, 1831, 1276), "16.2K tok    1.1s    11:46:09"),
    ]
    for box, value in recent_rows:
        replace_text(draw, box, value, color=MUTED, size=15, inset=(8, 5))

    return image


def sanitize_accounts(image: Image.Image) -> Image.Image:
    image = image.copy()
    draw = ImageDraw.Draw(image)
    rows = [346, 532, 716, 899, 1083]
    demo_values = [
        ("2.4M", "$12.8400"),
        ("1.8M", "$9.4200"),
        ("860K", "$4.7300"),
        ("24K", "$0.1200"),
        ("0", "$0.0000"),
    ]
    demo_usage = [(18, 42), (31, 58), (47, 73), (9, 22), (0, 0)]

    for index, (top, values, usage) in enumerate(zip(rows, demo_values, demo_usage), start=1):
        draw.rounded_rectangle((447, top + 59, 500, top + 111), radius=12, fill=ORANGE)
        draw.text(
            (473, top + 84),
            f"{index:02d}",
            fill=WHITE,
            font=font(17, bold=True),
            anchor="mm",
        )
        replace_text(
            draw,
            (510, top + 27, 835, top + 68),
            f"Demo Account {index:02d}",
            size=20,
            bold=True,
            inset=(5, 3),
        )
        replace_text(
            draw,
            (510, top + 105, 835, top + 145),
            "\u5e76\u53d1 -- \u00b7 \u6392\u961f --",
            color=MUTED,
            size=16,
            inset=(5, 4),
        )
        replace_text(
            draw,
            (850, top + 91, 1095, top + 126),
            "\u6700\u8fd1\u6210\u529f \u00b7 --",
            color=MUTED,
            size=16,
            inset=(3, 4),
        )
        draw.rectangle((1098, top + 25, 1397, top + 139), fill=WHITE)
        usage_labels = ["\u7528\u91cf\u9650\u989d", "7 \u5929\u7a97\u53e3\u7528\u91cf"]
        for usage_index, (label, percent) in enumerate(zip(usage_labels, usage)):
            offset = usage_index * 57
            draw.text((1103, top + 29 + offset), label, fill=MUTED, font=font(15))
            draw.text(
                (1391, top + 30 + offset),
                f"{percent:.1f}%",
                fill=(105, 101, 94),
                font=font(13, bold=True),
                anchor="rt",
            )
            bar_top = top + 66 + offset
            draw.rounded_rectangle((1103, bar_top, 1391, bar_top + 7), radius=3, fill=(235, 232, 224))
            if percent:
                bar_width = max(7, round(288 * percent / 100))
                draw.rounded_rectangle((1103, bar_top, 1103 + bar_width, bar_top + 7), radius=3, fill=ORANGE)
        replace_text(
            draw,
            (1400, top + 47, 1574, top + 82),
            values[0],
            size=18,
            bold=True,
            inset=(5, 2),
        )
        replace_text(
            draw,
            (1400, top + 112, 1574, top + 151),
            values[1],
            size=18,
            bold=True,
            inset=(5, 3),
        )

    return image


def sanitize_network(image: Image.Image) -> Image.Image:
    image = image.copy()
    draw = ImageDraw.Draw(image)
    draw.rectangle((422, 218, 800, 278), fill=PAGE)
    draw.text((427, 220), "\u8d26\u6237\u4ee3\u7406\u914d\u7f6e", fill=MUTED, font=font(14))
    draw.text(
        (427, 242),
        "\u6df7\u5408\u914d\u7f6e \u00b7 \u5df2\u7ed1\u5b9a 2 / 3",
        fill=INK,
        font=font(18, bold=True),
    )
    draw.rectangle((494, 475, 780, 558), fill=WHITE)
    replacements = [
        ((494, 349, 770, 390), "\u672c\u5730\u4ee3\u7406 01", WHITE, INK, 21, True),
        ((494, 392, 780, 427), "127.0.0.1:0000", WHITE, MUTED, 18, False),
        ((494, 486, 780, 522), "\u672c\u5730\u4ee3\u7406 02", WHITE, INK, 21, True),
        ((494, 522, 780, 558), "127.0.0.1:0000", WHITE, MUTED, 18, False),
        ((494, 423, 780, 454), "\u5df2\u7ed1\u5b9a 2 \u4e2a\u8d26\u6237", WHITE, MUTED, 15, False),
        ((494, 558, 780, 589), "\u5df2\u7ed1\u5b9a 0 \u4e2a\u8d26\u6237", WHITE, MUTED, 15, False),
    ]
    for box, value, fill, color, size, bold in replacements:
        replace_text(draw, box, value, fill=fill, color=color, size=size, bold=bold, inset=(5, 3))
    return image


def sanitize_sharing(image: Image.Image) -> Image.Image:
    image = image.copy()
    draw = ImageDraw.Draw(image)

    draw.rounded_rectangle((370, 90, 434, 154), radius=14, fill=(248, 235, 227))
    draw.text((402, 121), "A", fill=ORANGE, font=font(22, bold=True), anchor="mm")
    replace_text(
        draw,
        (444, 86, 818, 129),
        "Demo Workspace",
        fill=PAGE,
        size=20,
        bold=True,
        inset=(7, 5),
    )
    replace_text(
        draw,
        (444, 130, 825, 170),
        "demo@amber.local \u00b7 \u7ba1\u7406\u5458",
        fill=PAGE,
        color=MUTED,
        size=16,
        inset=(7, 5),
    )
    replace_text(
        draw,
        (423, 475, 790, 523),
        "000 000 000",
        fill=SHARING_INPUT,
        size=31,
        bold=True,
        inset=(4, 0),
    )
    avatar_fill = (251, 236, 231)
    for avatar_box, label in [((405, 766, 451, 812), "A"), ((405, 842, 451, 888), "B")]:
        draw.ellipse(avatar_box, fill=avatar_fill)
        draw.text(
            ((avatar_box[0] + avatar_box[2]) // 2, (avatar_box[1] + avatar_box[3]) // 2),
            label,
            fill=ORANGE,
            font=font(16, bold=True),
            anchor="mm",
        )
    replace_text(draw, (453, 752, 825, 798), "Demo User 01", size=20, bold=True, inset=(6, 4))
    replace_text(draw, (453, 827, 825, 873), "Demo User 02", size=20, bold=True, inset=(6, 4))
    replace_text(draw, (456, 795, 820, 826), "-- / -- \u00b7 Demo RPM", color=MUTED, size=14, inset=(4, 3))
    replace_text(draw, (456, 870, 820, 901), "-- / -- \u00b7 Demo RPM", color=MUTED, size=14, inset=(4, 3))
    replace_text(draw, (1096, 702, 1138, 740), "--", size=18, inset=(5, 3))
    for box in [(392, 1070, 700, 1118), (766, 1070, 1060, 1118), (1138, 1070, 1430, 1118)]:
        replace_text(draw, box, "--", fill=PAGE, size=22, bold=True, inset=(6, 4))
    replace_text(draw, (1510, 1076, 1835, 1122), "--", fill=PAGE, size=20, bold=True, inset=(6, 4))
    return image


def sanitize_codex(image: Image.Image) -> Image.Image:
    image = image.copy()
    draw = ImageDraw.Draw(image)

    replace_text(
        draw,
        (518, 326, 811, 371),
        r"C:\Users\User\.codex\config.toml",
        color=MUTED,
        size=17,
        inset=(5, 5),
    )
    replace_text(draw, (983, 326, 1150, 371), "--", color=MUTED, size=17, inset=(5, 5))
    replace_text(
        draw,
        (503, 740, 940, 783),
        r"C:\Users\User\.codex\config.toml",
        color=MUTED,
        size=17,
        inset=(5, 5),
    )
    replace_text(
        draw,
        (486, 821, 910, 865),
        r"C:\Users\User\.codex\auth.json",
        color=MUTED,
        size=17,
        inset=(5, 5),
    )
    return image


def crop_and_save(
    image: Image.Image,
    crop: tuple[int, int, int, int],
    path: Path,
    size: tuple[int, int] | None = None,
) -> None:
    result = image.crop(crop)
    if size is not None and result.size != size:
        result = result.resize(size, Image.Resampling.LANCZOS)
    result.save(path, "PNG", optimize=True)


def rounded_paste(base: Image.Image, overlay: Image.Image, position: tuple[int, int], radius: int) -> None:
    mask = Image.new("L", overlay.size, 0)
    ImageDraw.Draw(mask).rounded_rectangle((0, 0, overlay.width - 1, overlay.height - 1), radius=radius, fill=255)
    base.paste(overlay, position, mask)


def create_og_cover(dashboard: Image.Image, path: Path) -> None:
    cover = Image.new("RGB", (1200, 630), (244, 242, 236))
    draw = ImageDraw.Draw(cover)
    draw.rectangle((0, 0, 390, 630), fill=(36, 40, 37))

    icon_path = ROOT / "public" / "app-icon.png"
    icon = Image.open(icon_path).convert("RGBA").resize((58, 58), Image.Resampling.LANCZOS)
    cover.paste(icon, (56, 56), icon)
    draw.text((130, 58), "Amber", fill=WHITE, font=font(34, bold=True))
    draw.text((58, 154), "Windows Codex", fill=(229, 158, 122), font=font(20, bold=True))
    draw.multiline_text(
        (56, 202),
        "\u591a\u8d26\u53f7\u8c03\u5ea6\n\u597d\u53cb\u5171\u4eab\nCodex \u4e00\u952e\u63a5\u5165",
        fill=WHITE,
        font=font(37, bold=True),
        spacing=16,
    )
    draw.rounded_rectangle((56, 490, 196, 531), radius=6, fill=(207, 91, 55))
    draw.text((76, 498), "v0.4.4", fill=WHITE, font=font(18, bold=True))

    preview = dashboard.crop((326, 90, 1924, 1089)).resize((740, 463), Image.Resampling.LANCZOS)
    shadow = Image.new("RGBA", (788, 511), (0, 0, 0, 0))
    shadow_draw = ImageDraw.Draw(shadow)
    shadow_draw.rounded_rectangle((24, 24, 764, 487), radius=12, fill=(22, 25, 23, 72))
    shadow = shadow.filter(ImageFilter.GaussianBlur(16))
    cover.paste(shadow, (382, 60), shadow)
    rounded_paste(cover, preview, (404, 82), 10)
    path.parent.mkdir(parents=True, exist_ok=True)
    cover.save(path, "PNG", optimize=True)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Sanitize real Amber v0.4.4 product screenshots.")
    parser.add_argument("--dashboard", type=Path, required=True)
    parser.add_argument("--accounts", type=Path, required=True)
    parser.add_argument("--network", type=Path, required=True)
    parser.add_argument("--sharing", type=Path, required=True)
    parser.add_argument("--codex", type=Path, required=True)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    OUTPUT.mkdir(parents=True, exist_ok=True)

    dashboard = sanitize_dashboard(require_image(args.dashboard, SOURCE_HASHES["dashboard"]))
    accounts = sanitize_accounts(require_image(args.accounts, SOURCE_HASHES["accounts"]))
    network = sanitize_network(require_image(args.network, SOURCE_HASHES["network"]))
    sharing = sanitize_sharing(require_image(args.sharing, SOURCE_HASHES["sharing"]))
    codex = sanitize_codex(require_image(args.codex, SOURCE_HASHES["codex"]))

    full_crop = (2, 45, 1922, 1245)
    compact_crop = (326, 45, 1924, 1044)

    assets = [
        (dashboard, "dashboard-v044.png"),
        (accounts, "accounts-v044.png"),
        (network, "network-v044.png"),
        (sharing, "cloud-sharing-v044.png"),
        (codex, "codex-injection-v044.png"),
    ]
    for image, filename in assets:
        crop_and_save(image, full_crop, OUTPUT / filename)
        if filename != "dashboard-v044.png":
            compact_name = filename.replace(".png", "-compact.png")
            crop_and_save(image, compact_crop, OUTPUT / compact_name, (1440, 900))

    crop_and_save(dashboard, (2, 45, 1922, 645), OUTPUT / "hero-cover-v044.png")
    crop_and_save(dashboard, (326, 45, 1924, 1110), OUTPUT / "hero-cover-v044-mobile.png", (1080, 720))
    create_og_cover(dashboard, ROOT / "public" / "og-cover-v044.png")


if __name__ == "__main__":
    main()
