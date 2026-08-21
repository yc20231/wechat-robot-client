# 365 天安全主题库实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立一套只覆盖阳强塑料工厂真实现场、365 天标题和完整文案不重复、固定均匀轮换且可自动校验与安全部署的每日安全主题库。

**Architecture:** 使用带分类、对象、阶段、风险和动作标签的结构化 JSON 作为唯一事实来源，由 Python 标准库生成器进行内容校验和确定性编排，再原子生成项目内置库、部署库和人工审阅清单。Go 运行时继续读取原有扁平 JSON 并按日期取模，不改变每日 09:00 的发布接口；服务器仅替换主题文件并保持天气功能关闭。

**Tech Stack:** Python 3 标准库（`json`、`argparse`、`difflib`、`tempfile`、`unittest`）、Go、JSON、Markdown、SSH、Docker。

## Global Constraints

- 常规主题必须正好 `365` 条，365 个标题和完整文案全部唯一。
- 允许同一重要原则在不同设备、阶段或异常场景重复强化，但不得仅替换同义词制造假主题。
- 内容只覆盖已确认的吹膜、制袋、造粒、上料、牵引、收卷、换卷、破碎清料、仓储、叉车装卸、配电、压缩空气、检修、消防疏散、个人防护、交接班、现场整理和应急处置。
- 禁止加入吊装、危化品、有限空间等现场不存在的作业。
- 固定全年轮换，不读取天气、月份或节假日；服务器 `weather_enabled` 必须保持 `false`。
- 每条保持一个 `focus`、正好三个 `points` 和一个 `slogan`；采用准确、易懂但不过度口语化的现场规范用语。
- 相同安全类别至少间隔 `3` 天，相同设备或区域至少间隔 `7` 天，管理类内容不得连续，连续两天不得使用相同核心动作。
- 不修改目标群、每日 `09:00` 发布时间、发布接口或已发送记录，不补发历史内容。
- 实现前先检查并保留 `scripts/safety-topics/gen.py`、`scripts/safety-topics/check-safety.py` 和 `.deploy/safety-reminder-topics-plastics.json` 的现有未提交修改。

---

## File Map

- Create `scripts/safety-topics/topics-source.json`: 365 条带维护标签的唯一源库。
- Modify `scripts/safety-topics/gen.py`: 解析、校验、确定性编排、相似度报告和原子输出。
- Create `scripts/safety-topics/test_gen.py`: 生成器单元测试。
- Create `docs/safety-reminder-topics-365.md`: 自动生成的全年审阅清单。
- Generate `pkg/templates/safetyreminder/topics.json`: Go 二进制内置运行库。
- Generate `.deploy/safety-reminder-topics-plastics.json`: 本地及服务器部署运行库。
- Modify `pkg/safetyreminder/content_test.go`: 365 天运行周期和文案约束测试。
- Modify `docs/wechat-bot-fnos-deploy.md`: 记录唯一源库、生成、审阅、部署和回退命令。

---

### Task 1: 建立可测试的主题生成与校验管线

**Files:**
- Modify: `scripts/safety-topics/gen.py`
- Create: `scripts/safety-topics/test_gen.py`

**Interfaces:**
- Consumes: `topics-source.json` 中的 `list[dict]`。
- Produces: `load_source(path: Path) -> list[dict]`、`validate_entries(entries: list[dict]) -> list[str]`、`arrange_entries(entries: list[dict]) -> list[dict]`、`runtime_entry(entry: dict) -> dict`、`similar_pairs(entries: list[dict], threshold: float = 0.82) -> list[tuple]`、`generate(source: Path, outputs: list[Path], review: Path) -> None`。

- [ ] **Step 1: 写生成器失败测试**

在 `test_gen.py` 使用 `unittest` 定义最小合法记录工厂，并覆盖数量、配额、字段、标题/全文唯一、禁用词、长度、相似度、类别间隔、对象间隔、动作相邻和确定性输出。核心断言如下：

```python
class GeneratorTests(unittest.TestCase):
    def test_rejects_non_365_source(self):
        errors = gen.validate_entries([entry(0)])
        self.assertIn("常规主题必须正好 365 条", errors)

    def test_rejects_duplicate_focus_and_copy(self):
        entries = valid_entries()
        entries[1]["focus"] = entries[0]["focus"]
        entries[2]["points"] = list(entries[0]["points"])
        entries[2]["slogan"] = entries[0]["slogan"]
        errors = gen.validate_entries(entries)
        self.assertTrue(any("标题重复" in error for error in errors))
        self.assertTrue(any("完整文案重复" in error for error in errors))

    def test_arrangement_is_deterministic_and_respects_gaps(self):
        first = gen.arrange_entries(valid_entries())
        second = gen.arrange_entries(valid_entries())
        self.assertEqual(first, second)
        self.assertEqual([], gen.validate_schedule(first))

    def test_failed_generation_leaves_existing_output_unchanged(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "topics.json"
            output.write_text('[{"existing": true}]', encoding="utf-8")
            with self.assertRaises(gen.ValidationError):
                gen.generate(invalid_source_path(directory), [output], Path(directory) / "review.md")
            self.assertEqual('[{"existing": true}]', output.read_text(encoding="utf-8"))
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `python3 -m unittest scripts/safety-topics/test_gen.py -v`

Expected: FAIL，因为新接口尚不存在。

- [ ] **Step 3: 实现结构、配额和文案校验**

保留当前无天气版本的用户改动，将内嵌 25 条 `T` 替换为源文件读取。定义这些常量：

```python
EXPECTED_COUNTS = {
    "生产设备与操作": 105,
    "消防与用电": 45,
    "仓储与物流": 45,
    "检修与能源隔离": 40,
    "个人防护与职业健康": 35,
    "现场环境与秩序": 35,
    "应急处置与班组管理": 35,
    "其他现场实际风险": 25,
}
FORBIDDEN_TERMS = ("吊装", "起重", "危化品", "有限空间")
MAX_LENGTHS = {"focus": 12, "point": 24, "slogan": 22}
REQUIRED_KEYS = {"category", "object", "stage", "risk", "action_keys", "focus", "points", "slogan"}
MANAGEMENT_CATEGORY = "应急处置与班组管理"
```

`validate_entries` 必须累积全部错误后一次报告，不在第一个错误处退出。完整文案唯一键使用 `(focus, tuple(points), slogan)`；每条 `points` 必须是长度为 3 的字符串列表，`action_keys` 必须是非空字符串列表。

- [ ] **Step 4: 实现固定编排和相似度报告**

`arrange_entries` 按源文件稳定序号进行确定性回溯排列，候选优先级为“当前已发布次数少的类别、距离上次出现更久的对象、源序号更小”。每次放置前验证：类别距离至少 3、对象距离至少 7、管理类不连续、与前一条 `action_keys` 无交集。无法完成 365 条时抛出 `ValidationError` 并列出剩余类别、对象及首个冲突原因。

`similar_pairs` 对去除标点后的 `focus + points + slogan` 使用 `difflib.SequenceMatcher`，比例达到 `0.82` 时加入报告，但不自动改写或阻止生成；完全重复仍由硬校验阻止。

- [ ] **Step 5: 实现校验后原子生成**

CLI 固定为：

```bash
python3 scripts/safety-topics/gen.py \
  --source scripts/safety-topics/topics-source.json \
  --output pkg/templates/safetyreminder/topics.json \
  --output .deploy/safety-reminder-topics-plastics.json \
  --review docs/safety-reminder-topics-365.md \
  --check
```

`--check` 比较磁盘产物和内存结果，不写文件，存在差异时返回非零。无 `--check` 时先完成全部验证，再将三个产物分别写入同目录临时文件，最后用 `Path.replace()` 替换正式文件。两个运行 JSON 只保留 `focus`、`points`、`slogan`。

- [ ] **Step 6: 运行生成器测试**

Run: `python3 -m unittest scripts/safety-topics/test_gen.py -v`

Expected: PASS，覆盖成功生成和失败不覆盖旧文件。

- [ ] **Step 7: 提交生成管线**

```bash
git add scripts/safety-topics/gen.py scripts/safety-topics/test_gen.py
git commit -m "feat: validate annual safety topic generation"
```

---

### Task 2: 编写并审查 365 条结构化现场内容

**Files:**
- Create: `scripts/safety-topics/topics-source.json`
- Create: `docs/safety-reminder-topics-365.md`
- Modify generated: `pkg/templates/safetyreminder/topics.json`
- Modify generated: `.deploy/safety-reminder-topics-plastics.json`

**Interfaces:**
- Consumes: Task 1 的字段和配额约束。
- Produces: 365 条完整源记录，以及两份字节一致的运行 JSON。

- [ ] **Step 1: 按风险矩阵建立 365 个实质不同的切入点**

先为每个类别列出 `object × stage × risk`，再写文案。生产设备类必须覆盖吹膜、制袋、造粒、上料、牵引、收卷、换卷、破碎和清料；其他类别只使用 Global Constraints 中确认的现场对象。源记录格式固定为：

```json
{
  "category": "生产设备与操作",
  "object": "吹膜机",
  "stage": "异常处理",
  "risk": "断膜后手伸入辊隙",
  "action_keys": ["停机", "确认停止", "工具清理", "点动低速"],
  "focus": "吹膜机断膜处理",
  "points": [
    "发现断膜先停机，确认辊筒停止转动",
    "缠膜使用工具清理，手不要伸入辊隙",
    "重新引膜时采用点动低速运行"
  ],
  "slogan": "断膜先停机，处理不冒险"
}
```

- [ ] **Step 2: 运行硬校验并逐项修正文案**

Run: `python3 scripts/safety-topics/gen.py --source scripts/safety-topics/topics-source.json --output pkg/templates/safetyreminder/topics.json --output .deploy/safety-reminder-topics-plastics.json --review docs/safety-reminder-topics-365.md`

Expected: 输出 `365 topics generated`、八类数量摘要和相似条目数量；任何硬错误均不得产生新运行文件。

- [ ] **Step 3: 人工审查相似内容**

打开 `docs/safety-reminder-topics-365.md` 的“相似内容复核”部分。逐对确认对象、阶段、风险和员工动作存在实质差异；仅换词的条目回到源库重写。重复执行生成器，直到报告中不存在需要修改的高相似条目。

- [ ] **Step 4: 审查类别样本和连续 30 天节奏**

从审阅清单中检查每类至少 5 条，并检查第 `1–30`、`121–150`、`336–365` 天三个连续窗口。确认没有现场外内容、管理类连续、同类扎堆、相邻动作重复或文案过度口语化。

- [ ] **Step 5: 验证产物稳定且一致**

```bash
python3 scripts/safety-topics/gen.py --source scripts/safety-topics/topics-source.json --output pkg/templates/safetyreminder/topics.json --output .deploy/safety-reminder-topics-plastics.json --review docs/safety-reminder-topics-365.md --check
cmp pkg/templates/safetyreminder/topics.json .deploy/safety-reminder-topics-plastics.json
jq 'length' pkg/templates/safetyreminder/topics.json
```

Expected: `--check` 和 `cmp` 返回 0，`jq` 输出 `365`。

- [ ] **Step 6: 提交内容源和生成产物**

```bash
git add scripts/safety-topics/topics-source.json docs/safety-reminder-topics-365.md pkg/templates/safetyreminder/topics.json .deploy/safety-reminder-topics-plastics.json
git commit -m "content: add 365-day factory safety library"
```

---

### Task 3: 将 Go 运行测试升级到 365 天周期

**Files:**
- Modify: `pkg/safetyreminder/content_test.go`

**Interfaces:**
- Consumes: Task 2 生成的扁平 365 条 JSON。
- Produces: 运行时对 365 天周期、文案格式和无天气标签的回归保护。

- [ ] **Step 1: 先更新失败测试**

将 `expected 120 topics` 改为 `expected 365 topics`。删除强制提醒以前缀词开头的断言，加入以下检查：

```go
if topic.Tag != "" {
    t.Errorf("topic %d unexpectedly contains weather tag %q", index+1, topic.Tag)
}
if len := utf8.RuneCountInString(topic.Focus); len > 12 {
    t.Errorf("topic %d focus is too long (%d): %s", index+1, len, topic.Focus)
}
for pointIndex, point := range topic.Points {
    if len := utf8.RuneCountInString(point); len > 24 {
        t.Errorf("topic %d point %d is too long (%d): %s", index+1, pointIndex+1, len, point)
    }
}
if len := utf8.RuneCountInString(topic.Slogan); len > 22 {
    t.Errorf("topic %d slogan is too long (%d): %s", index+1, len, topic.Slogan)
}
```

增加完整文案唯一 map，键为 `topic.Focus + "\x00" + strings.Join(topic.Points[:], "\x00") + "\x00" + topic.Slogan`；并将周期错误文案中的 `120-day` 改为 `365-day`。

- [ ] **Step 2: 运行 Go 测试确认旧库或旧断言会失败**

Run: `go test ./pkg/safetyreminder -run 'TestDefaultTopics' -count=1`

Expected: 在 Task 2 产物尚未就绪的执行顺序中因数量不等于 365 失败；若 Task 2 已完成，则新测试直接通过。

- [ ] **Step 3: 运行完整包测试**

Run: `go test ./pkg/safetyreminder -count=1`

Expected: PASS，包括同日稳定、连续日期不同、365 天无重复和第 366 天回到首条。

- [ ] **Step 4: 提交 Go 回归测试**

```bash
git add pkg/safetyreminder/content_test.go
git commit -m "test: enforce 365-day safety topic cycle"
```

---

### Task 4: 验证海报适配并更新维护文档

**Files:**
- Modify: `docs/wechat-bot-fnos-deploy.md`
- Read: `docs/safety-reminder-topics-365.md`
- Generate locally: `output/safety-topics/short.png`
- Generate locally: `output/safety-topics/medium.png`
- Generate locally: `output/safety-topics/long.png`

**Interfaces:**
- Consumes: Task 2 的 365 条运行库和现有 `cmd/safety-poster-preview`。
- Produces: 三种长度的实际渲染证据和可重复的维护说明。

- [ ] **Step 1: 找出长、中、短文案样本**

从源库按 `focus + points + slogan` 总字符数排序，选择最短、中位数和最长条目的数组索引。使用日期取模反推出分别命中三个索引的日期，并在审阅记录中写明主题标题与日期。

- [ ] **Step 2: 渲染三张真实海报**

对三个日期分别运行：

```bash
go run ./cmd/safety-poster-preview --date 2026-01-01 --topics .deploy/safety-reminder-topics-plastics.json --out output/safety-topics/short.png
```

执行时用 Step 1 算出的实际日期替换示例日期，并分别输出 `short.png`、`medium.png`、`long.png`。检查 PNG 均为 `1279×1706`，标题、三条提醒和标语没有截断、遮挡或不合理缩小；发现问题时优先精炼源文案，不修改已确认的海报视觉模板。

- [ ] **Step 3: 更新飞牛维护说明**

在每日安全提醒章节加入：唯一源文件路径、生成命令、`--check` 命令、两份运行 JSON 必须一致、天气保持关闭、服务器备份/替换/回退和未来 30 天抽查命令。不得记录 SSH 私钥、Token 或群 ID。

- [ ] **Step 4: 运行本地交付门禁**

```bash
python3 -m unittest scripts/safety-topics/test_gen.py -v
python3 scripts/safety-topics/gen.py --source scripts/safety-topics/topics-source.json --output pkg/templates/safetyreminder/topics.json --output .deploy/safety-reminder-topics-plastics.json --review docs/safety-reminder-topics-365.md --check
cmp pkg/templates/safetyreminder/topics.json .deploy/safety-reminder-topics-plastics.json
go test ./pkg/safetyreminder -count=1
git diff --check
```

Expected: 所有命令返回 0。

- [ ] **Step 5: 提交维护文档**

```bash
git add docs/wechat-bot-fnos-deploy.md
git commit -m "docs: document annual safety topic maintenance"
```

---

### Task 5: 安全部署主题库并验证未来 30 天

**Files:**
- Source: `.deploy/safety-reminder-topics-plastics.json`
- Modify (server): `/vol1/1000/wechat-robot/jiqiren/.deploy/local/wechat-robot/xiW55bPyM3D4o6s6/data/skills/topics-plastics-365.json`
- Read (server): `/vol1/1000/wechat-robot/jiqiren/.deploy/local/wechat-robot/xiW55bPyM3D4o6s6/data/skills/.safety-reminder.json`
- Create (server): timestamped backup beside `topics-plastics-365.json`

**Interfaces:**
- Consumes: 通过全部本地门禁的部署 JSON。
- Produces: 服务器上的 365 条固定轮换库，不改变现有定时任务配置。

- [ ] **Step 1: 只读核对服务器配置和时间**

```bash
ssh feiniu-codex 'date; jq "{enabled, cron, send_on_weekends, weather_enabled, topics_file, target_count: (.target_chat_room_ids | length)}" /vol1/1000/wechat-robot/jiqiren/.deploy/local/wechat-robot/xiW55bPyM3D4o6s6/data/skills/.safety-reminder.json'
```

Expected: `enabled` 为 `true`、`cron` 为 `0 9 * * *`、`weather_enabled` 为 `false`，并且 `topics_file` 指向目标文件。输出不得包含 Token 或群 ID。

- [ ] **Step 2: 上传临时文件并核对哈希和条数**

```bash
scp .deploy/safety-reminder-topics-plastics.json feiniu-codex:/tmp/topics-plastics-365.json.new
shasum -a 256 .deploy/safety-reminder-topics-plastics.json
ssh feiniu-codex 'sha256sum /tmp/topics-plastics-365.json.new; jq length /tmp/topics-plastics-365.json.new'
```

Expected: 本地与服务器 SHA-256 相同，条数为 `365`。

- [ ] **Step 3: 备份并原子替换服务器主题库**

避开北京时间 `08:58–09:02` 执行。通过同一 SSH 会话设置明确路径、备份旧文件，再安装临时文件：

```bash
ssh feiniu-codex 'set -eu
target=/vol1/1000/wechat-robot/jiqiren/.deploy/local/wechat-robot/xiW55bPyM3D4o6s6/data/skills/topics-plastics-365.json
backup="${target}.pre-365-$(date +%Y%m%d-%H%M%S)"
cp -a "$target" "$backup"
install -m 600 /tmp/topics-plastics-365.json.new "${target}.new"
mv "${target}.new" "$target"
printf "%s\n" "$backup"'
```

保留命令输出的准确备份路径用于回退，不删除备份。

- [ ] **Step 4: 在服务器验证解析和未来 30 天**

```bash
ssh feiniu-codex 'python3 /vol1/1000/wechat-robot/jiqiren/scripts/safety-topics/check-safety.py 2026-08-22; jq length /vol1/1000/wechat-robot/jiqiren/.deploy/local/wechat-robot/xiW55bPyM3D4o6s6/data/skills/topics-plastics-365.json'
```

再对从服务器当前日期起的连续 30 天运行 `check-safety.py`，确认标题不重复、没有天气插播文本，并抽查首日、第 15 日和第 30 日内容。主题文件由每次 Cron 执行时读取，因此只替换 JSON 不重启客户端，避免影响当日任务注册和去重状态。

```bash
ssh feiniu-codex 'python3 - <<'"'"'PY'"'"'
import datetime
import json

path = "/vol1/1000/wechat-robot/jiqiren/.deploy/local/wechat-robot/xiW55bPyM3D4o6s6/data/skills/topics-plastics-365.json"
topics = json.load(open(path, encoding="utf-8"))
start = datetime.date.today()
seen = set()
for offset in range(30):
    day = start + datetime.timedelta(days=offset)
    unix_day = int(datetime.datetime(day.year, day.month, day.day, tzinfo=datetime.timezone.utc).timestamp()) // 86400
    focus = topics[unix_day % len(topics)]["focus"]
    if focus in seen:
        raise SystemExit(f"30 天内标题重复: {focus}")
    seen.add(focus)
    print(day.isoformat(), focus)
PY'
```

- [ ] **Step 5: 验证下一次任务并记录回退方法**

确认 `.safety-reminder.json` 的启用状态、Cron、群数量和 `weather_enabled: false` 未变化。若解析、未来 30 天或下一次实际发布出现异常，用 Step 3 输出的备份路径执行同目录 `cp -a backup target`，然后再次运行 `check-safety.py`；不补发失败或错过的历史提醒。

- [ ] **Step 6: 最终仓库检查**

```bash
git status --short
git log -5 --oneline
```

Expected: 本功能的源库、生成器、产物、测试和文档均已提交；任务开始前存在的无关工作区改动仍被保留且未混入这些提交。
