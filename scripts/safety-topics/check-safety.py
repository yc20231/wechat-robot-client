#!/usr/bin/env python3
"""查看某天安全提醒将发送的内容。用法: python3 check-safety.py [YYYY-MM-DD]"""
import json, re, sys, urllib.request, datetime

TOPICS = '/vol1/1000/wechat-robot/jiqiren/.deploy/local/wechat-robot/xiW55bPyM3D4o6s6/data/skills/topics-plastics-365.json'
CITY = '101230501'  # 泉州

date_str = sys.argv[1] if len(sys.argv) > 1 else datetime.date.today().isoformat()
d = datetime.date.fromisoformat(date_str)
topics = json.load(open(TOPICS))
normal = [t for t in topics if not t.get('tag')]
unixday = int(datetime.datetime(d.year, d.month, d.day, tzinfo=datetime.timezone.utc).timestamp()) // 86400

kind, weather_desc = '', ''
if d == datetime.date.today():
    try:
        req = urllib.request.Request(f'http://d1.weather.com.cn/dingzhi/{CITY}.html',
                                     headers={'Referer': 'http://www.weather.com.cn/'})
        text = urllib.request.urlopen(req, timeout=8).read().decode('utf-8', 'ignore')
        m = re.search(r'var cityDZ\d+ =(\{.*?\});', text)
        info = json.loads(m.group(1))['weatherinfo'] if m else {}
        ma = re.search(r'var alarmDZ\d+ =(\{.*?\});', text)
        alarms = json.loads(ma.group(1)).get('w', []) if ma else []
        wt, temp = info.get('weather', ''), info.get('temp', '')
        atext = ' '.join(a.get('w5', '') for a in alarms)
        combined = wt + ' ' + atext
        tempnum = int(temp.rstrip('℃度')) if temp.rstrip('℃度').isdigit() else 0
        if any(k in combined for k in ('台风', '暴雨', '大暴雨', '雷', '大雨')):
            kind = 'rain'
        elif '高温' in combined:
            kind = 'heat'
        elif '雨' in wt:
            kind = 'rain'
        elif tempnum >= 35:
            kind = 'heat'
        weather_desc = f"{info.get('cityname', '')} {wt} 最高{temp}" + (f" 预警: {atext}" if alarms else '')
    except Exception as e:
        weather_desc = f'天气查询失败: {e}'

if kind:
    pool = [t for t in topics if t.get('tag') == kind]
    pick = pool[unixday % len(pool)]
    src = f'天气插播({ "台风暴雨" if kind=="rain" else "高温" })'
else:
    pick = normal[unixday % len(normal)]
    src = '常规轮换'

print(f'日期: {date_str}  来源: {src}')
if weather_desc:
    print(f'当日天气: {weather_desc}')
print(f'今日重点: {pick["focus"]}')
for p in pick['points']:
    print(f'  - {p}')
print(f'标语: {pick["slogan"]}')
if not weather_desc:
    print('(按日期推算; 若当天遇雨/台风/高温将自动改为对应天气专题)')
