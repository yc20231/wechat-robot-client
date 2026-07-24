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
)
python3 -m py_compile "$stage_dir/scripts/text_to_image.py"

cp -p "$skill_dir/SKILL.md" "$backup_dir/SKILL.md"
cp -p "$skill_dir/scripts/text_to_image.py" "$backup_dir/scripts/text_to_image.py"

install -m 0644 "$stage_dir/SKILL.md" "$skill_dir/SKILL.md"
install -m 0755 "$stage_dir/scripts/text_to_image.py" "$skill_dir/scripts/text_to_image.py"

echo "Skill 已更新: $skill_dir"
echo "备份目录: $backup_dir"
