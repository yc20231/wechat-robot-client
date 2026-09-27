"""Enable the deployed production notice bridge; never submit/send a notice."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import urllib.request

BASE = Path('/vol1/1000/wechat-robot/jiqiren')
GROUP = '50005522283@chatroom'
CLIENT = 'client_xiW55bPyM3D4o6s6'
GATEWAY = 'business-gateway-business-gateway-1'
STAMP = 'enable-notice-20260927'

def inspect(name):
    return json.loads(subprocess.check_output(['docker', 'inspect', name]))[0]

def env_of(d):
    return dict(x.split('=', 1) for x in d['Config']['Env'] if '=' in x)

def update_env(path, values):
    stat = path.stat()
    original = path.read_text()
    lines = [line for line in original.splitlines() if not any(re.match(r'^\s*(?:export\s+)?' + re.escape(k) + r'\s*=', line) for k in values)]
    replacement = '\n'.join(lines) + '\n' + '\n'.join(k + '=' + v for k, v in values.items()) + '\n'
    tmp = path.with_name(path.name + '.notice-new')
    with tmp.open('x') as f:
        f.write(replacement)
    os.chmod(tmp, stat.st_mode & 0o777)
    os.chown(tmp, stat.st_uid, stat.st_gid)
    os.replace(tmp, path)

def erp_env():
    current = Path('/opt/erp-go/current/erp-api').resolve()
    pids = subprocess.check_output(['pgrep', '-x', 'erp-api'], text=True).split()
    pids = [p for p in pids if Path('/proc/' + p + '/exe').resolve() == current]
    assert len(pids) == 1
    return dict(x.split('=', 1) for x in Path('/proc/' + pids[0] + '/environ').read_bytes().decode().split('\0') if '=' in x)

def db_query(env, sql):
    def quote(v): return '"' + v.replace('\\', '\\\\').replace('"', '\\"').replace('\n', '\\n') + '"'
    with tempfile.TemporaryDirectory(prefix='notice-check-') as d:
        p = Path(d) / 'client.cnf'
        p.write_text('[client]\n' + '\n'.join(k + '=' + quote(env.get(v, default)) for k, v, default in [('host','DB_HOST','127.0.0.1'),('port','DB_PORT','3306'),('user','DB_USERNAME',''),('password','DB_PASSWORD','')]))
        p.chmod(0o600)
        return subprocess.check_output(['mysql', '--defaults-extra-file=' + str(p), '--batch', '--raw', '--skip-column-names', env['DB_DATABASE'], '-e', sql], stderr=subprocess.PIPE, text=True).strip()

def erp_enable():
    env = erp_env()
    prefix = env.get('DB_PREFIX', '')
    assert re.fullmatch(r'[A-Za-z0-9_]*', prefix)
    assert db_query(env, 'SELECT COUNT(*) FROM `' + prefix + 'production_notice_tasks`') == '0', 'Existing tasks need review before enabling'
    assert len(env.get('BOT_API_TOKEN', '')) >= 32 and not env.get('PRODUCTION_NOTICE_TOKEN')
    backup = Path('/opt/erp-go/backups') / STAMP
    backup.mkdir(mode=0o700, exist_ok=False)
    p = Path('/etc/erp-go/erp-go.env')
    shutil.copy2(p, backup / 'erp-go.env')
    update_env(p, {'PRODUCTION_NOTICE_REUSE_EXISTING_AUTH': 'true'})
    print(json.dumps({'configured': 'erp', 'queue_count_before': 0, 'backup': str(backup)}), flush=True)

def erp_verify():
    env = erp_env()
    assert env.get('PRODUCTION_NOTICE_REUSE_EXISTING_AUTH') == 'true'
    prefix = env.get('DB_PREFIX', '')
    assert re.fullmatch(r'[A-Za-z0-9_]*', prefix)
    raw = db_query(env, 'SELECT JSON_OBJECT(\'ready\',ready,\'heartbeat_age_seconds\',TIMESTAMPDIFF(SECOND,heartbeat_at,NOW()),\'bindings\',bindings) FROM `' + prefix + 'production_notice_bridge` WHERE id=1')
    bridge = json.loads(raw)
    if isinstance(bridge['bindings'], str): bridge['bindings'] = json.loads(bridge['bindings'])
    matched = [b for b in bridge['bindings'] if b.get('customer_code') == '270' and b.get('enabled')]
    assert bridge['ready'] == 1 and bridge['heartbeat_age_seconds'] < 30 and len(matched) == 1 and matched[0]['group_id'] == GROUP, 'Bridge not ready or binding mismatch'
    statuses = db_query(env, 'SELECT status,COUNT(*) FROM `' + prefix + 'production_notice_tasks` GROUP BY status')
    print(json.dumps({'erp_reuse_enabled': True, 'bridge': bridge, 'task_status_counts': statuses or 'empty'}, ensure_ascii=False))

def fnos_enable():
    c, g = inspect(CLIENT), inspect(GATEWAY)
    env = env_of(g)
    assert not env.get('PRODUCTION_NOTICE_TOKEN') and not env.get('PRODUCTION_NOTICE_ROBOT_TOKEN')
    assert len(env['BOT_TOKEN']) >= 32 and len(env['INTERNAL_ROUTE_TOKEN']) >= 32 and env['BOT_TOKEN'] != env['INTERNAL_ROUTE_TOKEN']
    expected_image = json.loads(subprocess.check_output(['docker', 'image', 'inspect', 'jiqiren/business-gateway:production-notice-20260927']))[0]['Id']
    assert json.loads(subprocess.check_output(['docker', 'image', 'inspect', 'jiqiren/business-gateway:local']))[0]['Id'] == expected_image
    skills = Path(next(m['Source'] for m in c['Mounts'] if m['Destination'] == '/data/skills'))
    volume = Path(next(m['Source'] for m in g['Mounts'] if m['Destination'] == '/data'))
    groups_path = volume / 'groups.json'
    original_groups = groups_path.read_bytes()
    groups = json.loads(original_groups)['groups']
    matches = [b for b in groups if b.get('customer_code') == '270' and b.get('enabled') and b.get('type') == 'customer']
    assert len(matches) == 1 and matches[0]['group_id'] == GROUP
    client_cfg = json.loads((skills / '.business-gateway.json').read_text())
    assert client_cfg['token'] == env['INTERNAL_ROUTE_TOKEN']
    backup = BASE / '.production-notice-deploy' / STAMP
    backup.mkdir(mode=0o700, exist_ok=False)
    shutil.copy2(BASE / 'business-gateway/.env', backup / 'gateway.env')
    shutil.copy2(groups_path, backup / 'groups.json')
    (backup / 'gateway-inspect.json').write_text(json.dumps(g))
    notice = skills / '.production-notice.json'
    assert not notice.exists(), 'Notice config already exists; review it first'
    notice.write_text('{"reuse_existing_auth":true}\n')
    notice.chmod(0o600)
    req = urllib.request.Request('http://' + c['NetworkSettings']['Networks']['wechat-robot']['IPAddress'] + ':9000/api/v1/robot/production-notice/health', headers={'X-Production-Notice-Token': env['INTERNAL_ROUTE_TOKEN']})
    with urllib.request.urlopen(req, timeout=10) as r:
        health = json.load(r)
    assert health.get('code') == 0 and health.get('data', {}).get('ready') is True
    update_env(BASE / 'business-gateway/.env', {
        'PRODUCTION_NOTICE_ENABLED':'true',
        'PRODUCTION_NOTICE_REUSE_EXISTING_AUTH':'true',
        'PRODUCTION_NOTICE_ROBOT_URL':'http://' + CLIENT + ':9000',
        'PRODUCTION_NOTICE_ALLOWED_GROUPS':GROUP,
    })
    try:
        subprocess.run(['docker','compose','config','-q'],cwd=BASE/'business-gateway',check=True)
        subprocess.run(['docker','compose','up','-d','--no-build','--no-deps','--force-recreate','business-gateway'],cwd=BASE/'business-gateway',check=True)
        new = inspect(GATEWAY)
        assert new['Image'] == expected_image
        new_env = env_of(new)
        assert all(new_env.get(k) == v for k, v in env.items() if not k.startswith('PRODUCTION_NOTICE_'))
        assert new['Mounts'] == g['Mounts']
        assert groups_path.read_bytes() == original_groups
    except Exception:
        shutil.copy2(backup/'gateway.env', BASE/'business-gateway/.env')
        subprocess.run(['docker','compose','up','-d','--no-build','--no-deps','--force-recreate','business-gateway'],cwd=BASE/'business-gateway',check=True)
        notice.write_text('{"reuse_existing_auth":false}\n')
        raise
    print(json.dumps({'enabled':'fnos','robot_ready':True,'allowed_group':GROUP,'existing_settings_preserved':True,'backup':str(backup)}),flush=True)

if __name__ == '__main__':
    os.umask(0o077)
    p=argparse.ArgumentParser()
    p.add_argument('step',choices=['erp_enable','erp_verify','fnos_enable'])
    globals()[p.parse_args().step]()
