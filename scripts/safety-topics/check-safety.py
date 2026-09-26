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
    print('(每天9:00按此表发送, 无天气干扰)')
