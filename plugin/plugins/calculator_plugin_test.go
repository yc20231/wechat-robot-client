package plugins

import (
	"math"
	"testing"
)

func TestParseCalculatorRequestDefaults(t *testing.T) {
	// 默认 HDPE 背心袋：总宽 = 30+5，长 50，丝数 2
	req, err := ParseCalculatorRequest("30+5*50/2 丝 查克数")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if req.MaterialKey != "hdpe" || req.BagKey != calculatorBagVest {
		t.Fatalf("默认公式错误: material=%s bag=%s", req.MaterialKey, req.BagKey)
	}
	if req.Width != 30 || req.Fold != 5 || req.Length != 50 || req.InputValue != 2 {
		t.Fatalf("解析数值错误: %+v", req)
	}
	if req.Mode != calculatorModeWeightFromSilk {
		t.Fatalf("计算方式错误: %s", req.Mode)
	}
}

func TestParseCalculatorRequestFlatBagWeight(t *testing.T) {
	req, err := ParseCalculatorRequest("@机器人 平口 30+5*50/6g 查丝数")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if req.BagKey != calculatorBagFlat {
		t.Fatalf("袋型错误: %s", req.BagKey)
	}
	if req.Mode != calculatorModeSilkFromWeight {
		t.Fatalf("计算方式错误: %s", req.Mode)
	}
	if req.InputValue != 6 {
		t.Fatalf("克重输入错误: %v", req.InputValue)
	}
}

func TestParseCalculatorRequestMentionAndFullWidthSymbols(t *testing.T) {
	req, err := ParseCalculatorRequest("@小助手 30×50／2克 查丝数")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if req.Width != 30 || req.Fold != 0 || req.Length != 50 || req.InputValue != 2 {
		t.Fatalf("解析数值错误: %+v", req)
	}
}

func TestParseCalculatorRequestTriggerOnly(t *testing.T) {
	if !parseCalculatorTrigger("30*50/2丝 查丝数") {
		t.Fatal("应识别出计算器触发词")
	}
	if parseCalculatorTrigger("今天天气怎么样") {
		t.Fatal("普通消息不应触发")
	}
}

func TestParseCalculatorRequestRejectsInvalid(t *testing.T) {
	cases := []string{
		"查克数",
		"30*50/0丝 查克数",
		"30*50/2克 查克数", // 单位是克但没有触发词错配不算错，此处仅验证可解析
	}
	for _, c := range cases {
		if _, err := ParseCalculatorRequest(c); err != nil {
			if c == "30*50/2克 查克数" {
				t.Fatalf("%q 应可解析: %v", c, err)
			}
			continue
		}
		if c == "查克数" {
			t.Fatalf("%q 不应解析成功", c)
		}
	}
}

func TestCalculateWeightFromSilk(t *testing.T) {
	// HDPE 背心袋，折边两侧各一道：总宽 = 30+5×2 = 40，40 × 50 × 2 ÷ 591 ≈ 6.77g
	req, err := ParseCalculatorRequest("30+5*50/2丝 查克数")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	result, err := req.Calculate()
	if err != nil {
		t.Fatalf("计算失败: %v", err)
	}
	if result.Formula.TemplateType != "hdpe_vest_bag" {
		t.Fatalf("公式错误: %s", result.Formula.TemplateType)
	}
	if result.TotalWidth != 40 {
		t.Fatalf("总宽应含两道折边=40，实际: %v", result.TotalWidth)
	}
	if math.Abs(result.SingleWeight-6.77) > 0.001 {
		t.Fatalf("克重计算错误: %v", result.SingleWeight)
	}
	if result.Silk != 2 {
		t.Fatalf("丝数错误: %v", result.Silk)
	}
}

func TestCalculateSilkFromWeightFlatBag(t *testing.T) {
	// HDPE 平口袋，折边×2：总宽 = 30+5×2 = 40，6 × 526.3 ÷ 40 ÷ 50 = 1.5789 → 四舍五入 2 丝
	req, err := ParseCalculatorRequest("平口 30+5*50/6克 查丝数")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	result, err := req.Calculate()
	if err != nil {
		t.Fatalf("计算失败: %v", err)
	}
	if result.Formula.TemplateType != "hdpe_flat_bag" {
		t.Fatalf("公式错误: %s", result.Formula.TemplateType)
	}
	if result.TotalWidth != 40 {
		t.Fatalf("总宽应含两道折边=40，实际: %v", result.TotalWidth)
	}
	if math.Abs(result.Silk-1.58) > 0.001 {
		t.Fatalf("丝数应保留两位小数=1.58，实际: %v", result.Silk)
	}
	if result.SingleWeight != 6 {
		t.Fatalf("克重错误: %v", result.SingleWeight)
	}
}

func TestResolveCalculatorFormulaKeys(t *testing.T) {
	cases := []struct {
		prefix   string
		material string
		bag      string
	}{
		{"", "hdpe", calculatorBagVest},
		{"平口 ", "hdpe", calculatorBagFlat},
		{"高压平口 ", "high_pressure", calculatorBagFlat},
		{"降解 ", "biodegradable_130", calculatorBagVest},
		{"降解1.26 ", "biodegradable_126", calculatorBagVest},
		{"HDPE ", "hdpe", calculatorBagVest},
	}
	for _, c := range cases {
		material, bag := resolveCalculatorFormulaKeys(c.prefix)
		if material != c.material || bag != c.bag {
			t.Fatalf("prefix=%q => material=%s bag=%s, 期望 %s/%s", c.prefix, material, bag, c.material, c.bag)
		}
		if calculatorFormulaFor(material, bag) == nil {
			t.Fatalf("material=%s bag=%s 未命中公式", material, bag)
		}
	}
}

func TestNormalizeCalculatorContent(t *testing.T) {
	got := normalizeCalculatorContent("@机器人 30＋5×50／2 丝 查克数")
	want := "30+5*50/2 丝 查克数"
	if got != want {
		t.Fatalf("归一化错误: got=%q want=%q", got, want)
	}
}

// 真实线上案例：微信 @分隔符是 U+2005，不是普通空格。
func TestParseCalculatorRequestWeChatMentionSeparator(t *testing.T) {
	// HDPE 背心袋，折边两侧各一道：总宽 = 30+7.5×2 = 45，45 × 50 × 1.5 ÷ 591 ≈ 5.71g
	req, err := ParseCalculatorRequest("@阳强机器人\u200530+7.5*50/1.5丝 查克数")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if req.Width != 30 || req.Fold != 7.5 || req.Length != 50 || req.InputValue != 1.5 {
		t.Fatalf("解析数值错误: %+v", req)
	}
	if req.MaterialKey != "hdpe" || req.BagKey != calculatorBagVest {
		t.Fatalf("默认公式错误: %s/%s", req.MaterialKey, req.BagKey)
	}
	result, err := req.Calculate()
	if err != nil {
		t.Fatalf("计算失败: %v", err)
	}
	if result.TotalWidth != 45 {
		t.Fatalf("总宽应含两道折边=45，实际: %v", result.TotalWidth)
	}
	if math.Abs(result.SingleWeight-5.71) > 0.001 {
		t.Fatalf("克重计算错误: %v", result.SingleWeight)
	}
}

// 不间断空格等其它 Unicode 空白也应兼容。
func TestParseCalculatorRequestTrailingNBSP(t *testing.T) {
	req, err := ParseCalculatorRequest("30*50/2丝 查克数\u00a0")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if req.Width != 30 || req.Length != 50 || req.InputValue != 2 {
		t.Fatalf("解析数值错误: %+v", req)
	}
}

// 对照计算器页“复制图片”输出的卡片文案。
func TestBuildCalculatorCardData(t *testing.T) {
	// HDPE 背心袋：宽20 折5 长33 丝1.5 → 总宽30，克重 2.51g
	req, err := ParseCalculatorRequest("20+5*33/1.5丝 查克数")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	result, err := req.Calculate()
	if err != nil {
		t.Fatalf("计算失败: %v", err)
	}
	data := buildCalculatorResultTemplateData(req, result)
	if data.MaterialLabel != "HDPE" || data.BagLabel != "背心袋" {
		t.Fatalf("袋子类型错误: %q - %q", data.MaterialLabel, data.BagLabel)
	}
	if data.SpecLine != "20+(5+5)*33cm" {
		t.Fatalf("规格错误: %q", data.SpecLine)
	}
	if data.WeightValue != "2.51" || data.SilkValue != "1.5" {
		t.Fatalf("数值错误: 克重=%q 丝数=%q", data.WeightValue, data.SilkValue)
	}

	// 无折边：平口 25*38 → 25*38cm
	req2, _ := ParseCalculatorRequest("平口 25*38/2.5丝 查克数")
	result2, _ := req2.Calculate()
	data2 := buildCalculatorResultTemplateData(req2, result2)
	if data2.MaterialLabel != "HDPE" || data2.BagLabel != "平口袋" {
		t.Fatalf("袋子类型错误: %q - %q", data2.MaterialLabel, data2.BagLabel)
	}
	if data2.SpecLine != "25*38cm" {
		t.Fatalf("规格错误: %q", data2.SpecLine)
	}
}

// 触发词内嵌公式名：查平口克数、查高压丝数、查降解1.26克数 等。
func TestParseCalculatorRequestTriggerFormula(t *testing.T) {
	cases := []struct {
		content  string
		material string
		bag      string
	}{
		{"30+5*50/2丝 查平口克数", "hdpe", calculatorBagFlat},
		{"30+5*50/2丝 查平口丝数", "hdpe", calculatorBagFlat},
		{"30+5*50/2丝 查高压丝数", "high_pressure", calculatorBagVest},
		{"30+5*50/2丝 查高压克数", "high_pressure", calculatorBagVest},
		{"30+5*50/2丝 查降解克数", "biodegradable_130", calculatorBagVest},
		{"30+5*50/2丝 查降解丝数", "biodegradable_130", calculatorBagVest},
		{"30+5*50/2丝 查降解1.26克数", "biodegradable_126", calculatorBagVest},
		{"30+5*50/2丝 查降解1.26丝数", "biodegradable_126", calculatorBagVest},
		{"30+5*50/2丝 查克数", "hdpe", calculatorBagVest},
		{"30+5*50/2丝 查丝数", "hdpe", calculatorBagVest},
	}
	for _, c := range cases {
		req, err := ParseCalculatorRequest(c.content)
		if err != nil {
			t.Fatalf("%q 解析失败: %v", c.content, err)
		}
		if req.MaterialKey != c.material || req.BagKey != c.bag {
			t.Fatalf("%q => %s/%s, 期望 %s/%s", c.content, req.MaterialKey, req.BagKey, c.material, c.bag)
		}
	}
	// 旧的前缀写法仍然兼容
	req, err := ParseCalculatorRequest("平口 30+5*50/6g 查丝数")
	if err != nil || req.MaterialKey != "hdpe" || req.BagKey != calculatorBagFlat {
		t.Fatalf("前缀写法应保持兼容: %v %+v", err, req)
	}
}

// 触发词后面带公式名：查丝数 平口袋、查克数 高压 等。
func TestParseCalculatorRequestTailFormula(t *testing.T) {
	cases := []struct {
		content  string
		material string
		bag      string
	}{
		{"30+5*50/2丝 查丝数 平口袋", "hdpe", calculatorBagFlat},
		{"30+5*50/2丝 查克数 高压", "high_pressure", calculatorBagVest},
		{"30+5*50/2丝 查克数 降解1.26", "biodegradable_126", calculatorBagVest},
		{"30+5*50/6克 查丝数 降解", "biodegradable_130", calculatorBagVest},
		{"30+5*50/2丝 查克数 背心袋", "hdpe", calculatorBagVest},
	}
	for _, c := range cases {
		req, err := ParseCalculatorRequest(c.content)
		if err != nil {
			t.Fatalf("%q 解析失败: %v", c.content, err)
		}
		if req.MaterialKey != c.material || req.BagKey != c.bag {
			t.Fatalf("%q => %s/%s, 期望 %s/%s", c.content, req.MaterialKey, req.BagKey, c.material, c.bag)
		}
	}
}

// 自动识别句式：关键词位置随意、无“查”字也可。
func TestParseCalculatorRequestLooseFormat(t *testing.T) {
	// 用户案例：背心100*140/50克 丝数 → 背心袋、克重50g反查丝数
	req, err := ParseCalculatorRequest("背心100*140/50克 丝数")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if req.BagKey != calculatorBagVest || req.MaterialKey != "hdpe" {
		t.Fatalf("公式错误: %s/%s", req.MaterialKey, req.BagKey)
	}
	if req.Width != 100 || req.Length != 140 || req.InputValue != 50 {
		t.Fatalf("数值错误: %+v", req)
	}
	if req.Mode != calculatorModeSilkFromWeight {
		t.Fatalf("计算方式错误: %s", req.Mode)
	}
	if !parseCalculatorTrigger("背心100*140/50克 丝数") {
		t.Fatal("应触发计算器")
	}
	// 无关键词的纯算式不触发
	if parseCalculatorTrigger("100*140/50克") {
		t.Fatal("纯算式不应触发")
	}
	if parseCalculatorTrigger("今天天气怎么样") {
		t.Fatal("普通消息不应触发")
	}
	// 旧的查X格式保持兼容
	for _, c := range []string{
		"30+5*50/2丝 查克数",
		"30+5*50/2丝 查平口克数",
		"30+5*50/2丝 查丝数 平口袋",
		"平口 30*50/2丝 查克数",
	} {
		if _, err := ParseCalculatorRequest(c); err != nil {
			t.Fatalf("%q 解析失败: %v", c, err)
		}
	}
}
