package plugins

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// 计算公式与 houduan 后端 database/schema/install.sql 中 product_formulas 种子数据保持一致：
// 求克重 = 总宽 × 长度 × 单层丝数 ÷ 常数 K；求丝数 = 克重 × K ÷ 总宽 ÷ 长度。
type calculatorFormula struct {
	TemplateType string
	MaterialKey  string
	BagKey       string
	Material     string
	ShortName    string // 卡片展示用短名，如 "HDPE"
	Bag          string
	Divisor      float64
}

const (
	calculatorBagVest = "vest"
	calculatorBagFlat = "flat"

	calculatorModeWeightFromSilk = "weight_from_silk"
	calculatorModeSilkFromWeight = "silk_from_weight"
)

var calculatorFormulas = map[string]*calculatorFormula{
	"hdpe_vest_bag": {
		TemplateType: "hdpe_vest_bag", MaterialKey: "hdpe", BagKey: calculatorBagVest,
		Material: "HDPE - 塑料袋", ShortName: "HDPE", Bag: "背心袋", Divisor: 591,
	},
	"hdpe_flat_bag": {
		TemplateType: "hdpe_flat_bag", MaterialKey: "hdpe", BagKey: calculatorBagFlat,
		Material: "HDPE - 塑料袋", ShortName: "HDPE", Bag: "平口袋", Divisor: 526.3,
	},
	"high_pressure_vest_bag": {
		TemplateType: "high_pressure_vest_bag", MaterialKey: "high_pressure", BagKey: calculatorBagVest,
		Material: "高压袋", ShortName: "高压", Bag: "背心袋", Divisor: 610.7,
	},
	"high_pressure_flat_bag": {
		TemplateType: "high_pressure_flat_bag", MaterialKey: "high_pressure", BagKey: calculatorBagFlat,
		Material: "高压袋", ShortName: "高压", Bag: "平口袋", Divisor: 543.5,
	},
	"biodegradable_130_vest_bag": {
		TemplateType: "biodegradable_130_vest_bag", MaterialKey: "biodegradable_130", BagKey: calculatorBagVest,
		Material: "全生物降解（密度1.3，填充30%）", ShortName: "全生物降解（密度1.3，填充30%）", Bag: "背心袋", Divisor: 432.15,
	},
	"biodegradable_130_flat_bag": {
		TemplateType: "biodegradable_130_flat_bag", MaterialKey: "biodegradable_130", BagKey: calculatorBagFlat,
		Material: "全生物降解（密度1.3，填充30%）", ShortName: "全生物降解（密度1.3，填充30%）", Bag: "平口袋", Divisor: 384.62,
	},
	"biodegradable_126_vest_bag": {
		TemplateType: "biodegradable_126_vest_bag", MaterialKey: "biodegradable_126", BagKey: calculatorBagVest,
		Material: "全生物降解（密度1.26，少量填充）", ShortName: "全生物降解（密度1.26，少量填充）", Bag: "背心袋", Divisor: 445.87,
	},
	"biodegradable_126_flat_bag": {
		TemplateType: "biodegradable_126_flat_bag", MaterialKey: "biodegradable_126", BagKey: calculatorBagFlat,
		Material: "全生物降解（密度1.26，少量填充）", ShortName: "全生物降解（密度1.26，少量填充）", Bag: "平口袋", Divisor: 396.83,
	},
}

func calculatorFormulaFor(materialKey, bagKey string) *calculatorFormula {
	return calculatorFormulas[materialKey+"_"+bagKey+"_bag"]
}

// CalculatorRequest 一条微信消息解析出的计算请求。
type CalculatorRequest struct {
	MaterialKey string
	BagKey      string
	Width       float64 // 总宽（不含折边）
	Fold        float64 // 折边，可为 0
	Length      float64
	InputValue  float64
	Mode        string // calculatorModeWeightFromSilk：输入丝数求克重；calculatorModeSilkFromWeight：输入克重求丝数
}

// CalculatorResult 计算结果。
type CalculatorResult struct {
	Formula      *calculatorFormula
	Mode         string
	TotalWidth   float64
	Length       float64
	Silk         float64
	SingleWeight float64
}

var (
	calculatorMentionPattern = regexp.MustCompile(`^@\S+\s+`)
	// 表达式：宽[+折]*长/数值+单位，允许出现在句子任意位置
	calculatorExprPattern = regexp.MustCompile(`(\d+(?:\.\d+)?)(?:\+(\d+(?:\.\d+)?))?\s*[x*]\s*(\d+(?:\.\d+)?)\s*/\s*(\d+(?:\.\d+)?)\s*(丝|克|g)`)
	// 计算意图关键词：查询词（克数/丝数）或公式关键词（平口/背心/高压/降解…）
	calculatorInterestPattern = regexp.MustCompile(`(?i)克数|克重|丝数|平口|平袋|背心|笑脸|高压|降解|hdpe`)
	calculatorContentReplacer = strings.NewReplacer(
		"×", "*", "＊", "*", "x", "*", "X", "*",
		"÷", "/", "／", "/",
		"＋", "+", "　", " ",
	)
)

// normalizeCalculatorContent 先把 U+2005（微信 @分隔符）等 Unicode 空白统一为普通空格，
// 再去掉群聊 @前缀 并把常见数学符号归一化为半角。
func normalizeCalculatorContent(content string) string {
	text := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, strings.TrimSpace(content))
	for calculatorMentionPattern.MatchString(text) {
		text = strings.TrimSpace(calculatorMentionPattern.ReplaceAllString(text, ""))
	}
	return calculatorContentReplacer.Replace(text)
}

// parseCalculatorTrigger 判断消息是否在尝试使用计算器：
// 需同时含有算式（如 100*140/50克）和 计算关键词（克数/丝数/平口/背心/高压/降解 等）。
func parseCalculatorTrigger(content string) bool {
	text := normalizeCalculatorContent(content)
	if !calculatorExprPattern.MatchString(text) {
		return false
	}
	return calculatorInterestPattern.MatchString(text)
}

// ParseCalculatorRequest 自动识别句式里的算式与关键词，
// "背心100*140/50克 丝数""30+5*50/2丝 查平口克数""平口 30*50/2丝 查克数"均可识别。
// 材料默认 HDPE、袋型默认背心袋；+折边 写法会折算进总宽（折边×2）。
func ParseCalculatorRequest(content string) (*CalculatorRequest, error) {
	text := normalizeCalculatorContent(content)
	loc := calculatorExprPattern.FindStringSubmatchIndex(text)
	if loc == nil {
		return nil, fmt.Errorf("计算格式无法识别")
	}
	match := make([]string, len(loc)/2)
	for i := 0; i < len(loc)/2; i++ {
		if loc[2*i] < 0 {
			match[i] = ""
			continue
		}
		match[i] = text[loc[2*i]:loc[2*i+1]]
	}

	width, _ := strconv.ParseFloat(match[1], 64)
	fold, _ := strconv.ParseFloat(match[2], 64)
	length, _ := strconv.ParseFloat(match[3], 64)
	value, _ := strconv.ParseFloat(match[4], 64)
	if width <= 0 || length <= 0 || value <= 0 {
		return nil, fmt.Errorf("宽、长和数值都必须大于 0")
	}

	// 算式以外的文字（前缀+后缀）都作为关键词上下文识别
	context := strings.TrimSpace(text[:loc[0]] + " " + text[loc[1]:])
	materialKey, bagKey := resolveCalculatorFormulaKeys(context)

	mode := calculatorModeWeightFromSilk
	if match[5] == "克" || strings.EqualFold(match[5], "g") {
		mode = calculatorModeSilkFromWeight
	}

	return &CalculatorRequest{
		MaterialKey: materialKey,
		BagKey:      bagKey,
		Width:       width,
		Fold:        fold,
		Length:      length,
		InputValue:  value,
		Mode:        mode,
	}, nil
}

// calculatorMaterialFromText 从文字里识别材料关键词，未识别返回 ok=false。
func calculatorMaterialFromText(text string) (materialKey string, ok bool) {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "降解"):
		if strings.Contains(lower, "1.26") || strings.Contains(lower, "126") {
			return "biodegradable_126", true
		}
		return "biodegradable_130", true
	case strings.Contains(lower, "高压"):
		return "high_pressure", true
	case strings.Contains(lower, "hdpe"):
		return "hdpe", true
	}
	return "", false
}

// calculatorBagFromText 从文字里识别袋型关键词，未识别返回 ok=false。
func calculatorBagFromText(text string) (bagKey string, ok bool) {
	if strings.Contains(text, "平口") || strings.Contains(text, "平袋") {
		return calculatorBagFlat, true
	}
	if strings.Contains(text, "背心") || strings.Contains(text, "笑脸") {
		return calculatorBagVest, true
	}
	return "", false
}

// resolveCalculatorFormulaKeys 从表达式前的文字里识别材料与袋型关键词，默认 HDPE 背心袋。
func resolveCalculatorFormulaKeys(prefix string) (materialKey, bagKey string) {
	materialKey, _ = calculatorMaterialFromText(prefix)
	if materialKey == "" {
		materialKey = "hdpe"
	}
	bagKey, _ = calculatorBagFromText(prefix)
	if bagKey == "" {
		bagKey = calculatorBagVest
	}
	return materialKey, bagKey
}

// Calculate 按后端同口径计算：克重保留两位小数，反查丝数四舍五入取整。
func (r *CalculatorRequest) Calculate() (*CalculatorResult, error) {
	formula := calculatorFormulaFor(r.MaterialKey, r.BagKey)
	if formula == nil {
		return nil, fmt.Errorf("暂不支持该公式，可用：HDPE/高压/降解 + 背心袋/平口袋")
	}
	totalWidth := r.Width + r.Fold*2 // 折边在袋子两侧各一道，按两倍折算进总宽
	if totalWidth <= 0 || r.Length <= 0 {
		return nil, fmt.Errorf("总宽和长度必须大于 0")
	}
	result := &CalculatorResult{
		Formula:    formula,
		Mode:       r.Mode,
		TotalWidth: totalWidth,
		Length:     r.Length,
	}
	if r.Mode == calculatorModeWeightFromSilk {
		result.Silk = calculatorRound(r.InputValue, 2)
		result.SingleWeight = calculatorRound(totalWidth*r.Length*r.InputValue/formula.Divisor, 2)
	} else {
		result.SingleWeight = r.InputValue
		// 与计算器页一致：反查丝数保留两位小数，不取整
		result.Silk = calculatorRound(r.InputValue*formula.Divisor/totalWidth/r.Length, 2)
	}
	if result.SingleWeight <= 0 || result.Silk <= 0 {
		return nil, fmt.Errorf("计算结果无效，请检查输入数值")
	}
	return result, nil
}

func calculatorRound(v float64, places int) float64 {
	p := math.Pow10(places)
	return math.Round(v*p) / p
}

// formatCalculatorNumber 去掉多余的小数尾零。
func formatCalculatorNumber(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

const calculatorUsageText = "塑料袋计算器用法（关键词位置随意）：\n" +
	"30+5*50/2丝 查克数 → 默认HDPE背心袋，算单个克重\n" +
	"背心100*140/50克 丝数 → 背心袋，已知克重50g，反查丝数\n" +
	"平口 100*140/2丝 → 平口袋公式\n" +
	"100*140/2丝 高压 / 降解 / 降解1.26 → 指定材料"
