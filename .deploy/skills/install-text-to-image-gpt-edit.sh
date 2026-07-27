#!/bin/sh

set -eu

if [ "$#" -ne 1 ]; then
  echo "用法: $0 /data/skills/text-to-image" >&2
  exit 2
fi

skill_dir="$1"
repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
patch_file="$repo_dir/.deploy/skills/text-to-image-gpt-edit.patch"
size_patch_file="$repo_dir/.deploy/skills/text-to-image-packy-size.patch"
auto_url_patch_file="$repo_dir/.deploy/skills/text-to-image-auto-url.patch"
edit_transport_file="$repo_dir/.deploy/skills/text_to_image_edit_transport.py"
upstream_revision="2a7cf121fdb9e05cec8dd1bebc916fff5016b741"
upstream_base="https://raw.githubusercontent.com/hp0912/wechat-robot-skills/$upstream_revision/skills/text-to-image"
stage_dir=$(mktemp -d "$repo_dir/.skill-stage.XXXXXX")
stage_relative=${stage_dir#"$repo_dir"/}
backup_dir="$repo_dir/.skill-backups/text-to-image-$(date +%Y%m%d-%H%M%S)"

cleanup() {
  rm -rf -- "$stage_dir"
}
trap cleanup EXIT HUP INT TERM

if [ ! -f "$skill_dir/SKILL.md" ] || [ ! -f "$skill_dir/scripts/text_to_image.py" ]; then
  echo "未找到已安装的 text-to-image Skill: $skill_dir" >&2
  exit 1
fi
mkdir -p "$stage_dir/scripts" "$backup_dir/scripts"
curl -fsSL "$upstream_base/SKILL.md" -o "$stage_dir/SKILL.md"
curl -fsSL "$upstream_base/scripts/text_to_image.py" -o "$stage_dir/scripts/text_to_image.py"

(
  cd "$repo_dir"
  git apply --check --directory="$stage_relative" "$patch_file"
  git apply --directory="$stage_relative" "$patch_file"
  git apply --check --directory="$stage_relative" "$size_patch_file"
  git apply --directory="$stage_relative" "$size_patch_file"
  git apply --check --directory="$stage_relative" "$auto_url_patch_file"
  git apply --directory="$stage_relative" "$auto_url_patch_file"
)
install -m 0644 "$edit_transport_file" "$stage_dir/scripts/text_to_image_edit_transport.py"
python3 -m py_compile "$stage_dir/scripts/text_to_image.py"
python3 -m py_compile "$stage_dir/scripts/text_to_image_edit_transport.py"

# The image workflow must follow the ChatGPT-style direct execution behavior.
# Fail closed if an upstream/patch combination reintroduces the legacy A/B gate.
if grep -Fq '你想要哪种风格？A. 白底商品图' "$stage_dir/SKILL.md"; then
  echo "检测到旧版图片 Skill 的 A/B 风格询问逻辑，拒绝安装" >&2
  exit 1
fi
if ! grep -Fq '不得询问 A/B' "$stage_dir/SKILL.md"; then
  echo "图片 Skill 缺少 ChatGPT 风格的直接执行规则，拒绝安装" >&2
  exit 1
fi

cp -p "$skill_dir/SKILL.md" "$backup_dir/SKILL.md"
cp -p "$skill_dir/scripts/text_to_image.py" "$backup_dir/scripts/text_to_image.py"
if [ -f "$skill_dir/scripts/text_to_image_edit_transport.py" ]; then
  cp -p "$skill_dir/scripts/text_to_image_edit_transport.py" "$backup_dir/scripts/text_to_image_edit_transport.py"
fi

install -m 0644 "$stage_dir/SKILL.md" "$skill_dir/SKILL.md"
install -m 0755 "$stage_dir/scripts/text_to_image.py" "$skill_dir/scripts/text_to_image.py"
install -m 0644 "$stage_dir/scripts/text_to_image_edit_transport.py" "$skill_dir/scripts/text_to_image_edit_transport.py"

echo "Skill 已更新: $skill_dir"
echo "备份目录: $backup_dir"
