from pathlib import Path

from PIL import Image, ImageOps


ASSET_DIR = Path(__file__).with_name("assets")
SOURCE = ASSET_DIR / "poster-background.png"
FACTORY_TOP = 900
FACTORY_BOTTOM = 1519
PRESERVED_BOXES = (
    (214, 956, 313, 1041),
    (214, 1052, 313, 1137),
    (214, 1148, 313, 1233),
    (510, 1235, 770, 1480),
)


def row_background(factory: Image.Image) -> Image.Image:
    """Build a clean row-wise backdrop for the narrow margins of scaled variants."""
    backdrop = Image.new("RGB", factory.size)
    source = factory.load()
    target = backdrop.load()
    width, height = factory.size
    for y in range(height):
        samples = [source[x, y] for x in range(0, width, 16)]
        samples.sort(key=lambda color: sum(color))
        color = samples[len(samples) * 3 // 4]
        for x in range(width):
            target[x, y] = color
    return backdrop


def scale_factory(factory: Image.Image, scale: float, mirror: bool = False) -> Image.Image:
    source = ImageOps.mirror(factory) if mirror else factory
    width, height = source.size
    resized_width = round(width * scale)
    resized = source.resize((resized_width, height), Image.Resampling.LANCZOS)
    if resized_width >= width:
        left = (resized_width - width) // 2
        return resized.crop((left, 0, left + width, height))

    result = row_background(factory)
    result.paste(resized, ((width - resized_width) // 2, 0))
    return result


def remove_preserved_artwork(factory: Image.Image) -> Image.Image:
    clean = factory.copy()
    backdrop = row_background(factory)
    for left, top, right, bottom in PRESERVED_BOXES:
        relative_box = (left, top - FACTORY_TOP, right, bottom - FACTORY_TOP)
        clean.paste(backdrop.crop(relative_box), relative_box[:2])
    return clean


def main() -> None:
    poster = Image.open(SOURCE).convert("RGB")
    factory = remove_preserved_artwork(
        poster.crop((0, FACTORY_TOP, poster.width, FACTORY_BOTTOM))
    )
    preserved_artwork = [(box, poster.crop(box)) for box in PRESERVED_BOXES]

    variants = {
        2: ImageOps.mirror(factory),
        3: scale_factory(factory, 0.94),
        4: scale_factory(factory, 1.06),
        5: scale_factory(factory, 0.96, mirror=True),
    }
    for number, variant_factory in variants.items():
        variant = poster.copy()
        variant.paste(variant_factory, (0, FACTORY_TOP))
        for box, artwork in preserved_artwork:
            variant.paste(artwork, (box[0], box[1]))
        output = ASSET_DIR / f"poster-background-{number}.png"
        variant.save(output, format="PNG", optimize=True)
        print(output)


if __name__ == "__main__":
    main()
