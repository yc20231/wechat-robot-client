"""Scoped fnOS release helper. Never enables notifications or sends messages."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import time
import urllib.request
import urllib.error

BASE = Path('/vol1/1000/wechat-robot/jiqiren')
RELEASE = BASE / '.production-notice-deploy' / '20260927-01'
CLIENT = 'client_xiW55bPyM3D4o6s6'
GATEWAY = 'business-gateway-business-gateway-1'

def run(args, **kw):
    return subprocess.check_output(args, **kw)

def inspect(name):
    return json.loads(run(['docker', 'inspect', name]))[0]

def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()

def extract(archive, target):
    with tarfile.open(archive) as t:
        for m in t.getmembers():
            p = Path(m.name)
            if p.is_absolute() or '..' in p.parts or not (m.isfile() or m.isdir()):
                raise RuntimeError('Unsafe archive member')
        t.extractall(target)

def prepare():
    os.umask(0o077)
    RELEASE.mkdir(parents=True, exist_ok=False)
    original = RELEASE / 'baseline'
    original.mkdir()
    extract('/tmp/production-notice-baseline-20260927.tar', original)
    for p in original.rglob('*'):
        if p.is_file() and p.read_text().replace('\r\n', '\n') != (BASE / p.relative_to(original)).read_text().replace('\r\n', '\n'):
            raise RuntimeError('Remote source changed: ' + str(p.relative_to(original)))
    payload = RELEASE / 'payload'
    payload.mkdir()
    extract('/tmp/production-notice-payload-20260927.tar', payload)
    backup = RELEASE / 'backup'
    backup.mkdir()
    client, gateway = inspect(CLIENT), inspect(GATEWAY)
    for name, obj in [('client', client), ('gateway', gateway)]:
        (backup / (name + '-inspect.json')).write_text(json.dumps(obj))
        run(['docker', 'tag', obj['Image'], 'jiqiren/' + ('wechat-robot-client' if name == 'client' else 'business-gateway') + ':rollback-before-notice-20260927'])
    env = dict(v.split('=', 1) for v in gateway['Config']['Env'] if '=' in v)
    assert env.get('PRODUCTION_NOTICE_ENABLED', '').lower() not in ('true', '1'), 'worker unexpectedly enabled'
    skills = Path(next(m['Source'] for m in client['Mounts'] if m['Destination'] == '/data/skills'))
    assert not (skills / '.production-notice.json').exists(), 'notice config already exists'
    cfgs = [BASE / 'business-gateway/.env', BASE / 'business-gateway/docker-compose.yml'] + list(skills.glob('.*.json'))
    with tarfile.open(backup / 'configs.tar.gz', 'w:gz') as t:
        for p in cfgs:
            t.add(p, arcname=str(p).lstrip('/'))
    volume = Path(next(m['Source'] for m in gateway['Mounts'] if m['Destination'] == '/data'))
    with tarfile.open(backup / 'gateway-data.tar.gz', 'w:gz') as t:
        t.add(volume, arcname='gateway-data')
    (backup / 'config-hashes.json').write_text(json.dumps({str(p): sha(p) for p in cfgs}))
    run(['docker', 'cp', CLIENT + ':/app/wechat-robot-client', str(backup / 'wechat-robot-client')])
    run(['docker', 'cp', GATEWAY + ':/app/business-gateway', str(backup / 'business-gateway')])
    source = RELEASE / 'source'
    def ignore(directory, names):
        return [n for n in names if n.startswith('.') or n in ('node_modules', 'output', 'tmp', 'dist') or n.startswith('wechat-robot-client-custom') or n.startswith('business-gateway-custom')]
    shutil.copytree(BASE, source, ignore=ignore)
    # Only task-owned files are overlaid; server custom source remains authoritative.
    for p in payload.rglob('*'):
        if p.is_file():
            dst = source / p.relative_to(payload)
            dst.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(p, dst)
    print(json.dumps({'prepared': str(RELEASE), 'configuration_backups': len(cfgs)}), flush=True)

def build():
    source = RELEASE / 'source'
    env = os.environ.copy()
    env.update(GOMAXPROCS='2', CGO_ENABLED='0', GOPROXY='https://goproxy.cn,direct', GOMODCACHE=str(BASE / '.tools/gomodcache'), GOCACHE=str(BASE / '.tools/gobuildcache'))
    go = str(BASE / '.tools/go/bin/go')
    commands = [
        (source, [go, 'test', './controller', './pkg/robot', '-run', '^(TestProductionNotice.*|TestValidateProductionNoticeReceipt)$', '-timeout', '60s']),
        (source, [go, 'test', './plugin/plugins', '-run', '^(TestBusinessRouter.*|TestLoadBusinessRouterConfigFromMountedFile|TestInvalidConfiguredBusinessRouterFailsClosed|TestAppendAIReplyFooter.*|TestParseCalculator.*|TestCalculate.*)$', '-timeout', '60s']),
        (source, [go, 'test', './pkg/safetyreminder', '-timeout', '60s']),
        (source / 'business-gateway', [go, 'test', './internal/config', './internal/notices', './internal/route', './internal/httpapi', '-timeout', '60s']),
        (source, [go, 'build', '-trimpath', '-ldflags=-s -w -X main.Version=production-notice-20260927', '-o', str(RELEASE / 'wechat-robot-client'), '.']),
        (source / 'business-gateway', [go, 'build', '-trimpath', '-ldflags=-s -w', '-o', str(RELEASE / 'business-gateway'), './cmd/business-gateway']),
    ]
    for cwd, command in commands:
        print('RUN', ' '.join(command[1:]), flush=True)
        subprocess.run(command, cwd=cwd, env=env, check=True)
    for name, binary, target in [('client', 'wechat-robot-client', '/app/wechat-robot-client'), ('gateway', 'business-gateway', '/app/business-gateway')]:
        info = json.loads((RELEASE / 'backup' / (name + '-inspect.json')).read_text())
        ctx = RELEASE / ('image-' + name)
        ctx.mkdir(exist_ok=True)
        shutil.copy2(RELEASE / binary, ctx / binary)
        base_tag = 'jiqiren/' + binary + ':rollback-before-notice-20260927'
        assert json.loads(run(['docker', 'image', 'inspect', base_tag]))[0]['Id'] == info['Image']
        (ctx / 'Dockerfile').write_text('FROM ' + base_tag + '\nCOPY --chmod=0755 ' + binary + ' ' + target + '\n')
        subprocess.run(['docker', 'build', '--pull=false', '--network=none', '-t', 'jiqiren/' + binary + ':production-notice-20260927', str(ctx)], check=True)
    (RELEASE / 'built.ok').write_text('ok')
    print('BUILD_OK', flush=True)

def http_status(url):
    try:
        with urllib.request.urlopen(url, timeout=5) as r:
            return r.status
    except urllib.error.HTTPError as e:
        return e.code

def replace(container, binary):
    target = '/app/' + binary.name
    run(['docker', 'cp', str(binary), container + ':' + target + '.notice-new'])
    run(['docker', 'exec', container, 'sh', '-c', 'chmod 755 ' + target + '.notice-new && mv ' + target + '.notice-new ' + target])
    run(['docker', 'restart', '-t', '20', container])

def verify():
    for p, expected in json.loads((RELEASE / 'backup/config-hashes.json').read_text()).items():
        assert sha(p) == expected, 'Configuration changed: ' + p
    for name, container, binary in [('client', CLIENT, 'wechat-robot-client'), ('gateway', GATEWAY, 'business-gateway')]:
        old = json.loads((RELEASE / 'backup' / (name + '-inspect.json')).read_text())
        now = inspect(container)
        assert now['Id'] == old['Id'] and now['State']['Running'], 'Container not preserved/running'
        assert now['Config']['Env'] == old['Config']['Env'] and now['Mounts'] == old['Mounts'], 'Runtime configuration changed'
        actual = run(['docker', 'exec', container, 'sha256sum', '/app/' + binary], text=True).split()[0]
        assert actual == sha(RELEASE / binary), 'Binary mismatch'
    c = inspect(CLIENT)
    ip = c['NetworkSettings']['Networks']['wechat-robot']['IPAddress']
    assert http_status('http://' + ip + ':9000/api/v1/robot/production-notice/health') == 401, 'Notice endpoint should be disabled'
    assert http_status('http://127.0.0.1:18080/healthz') == 200, 'Gateway unhealthy'
    print('VERIFY_OK: original container IDs/env/mounts/configs preserved; new binaries verified; notices disabled', flush=True)

def switch():
    assert (RELEASE / 'built.ok').exists()
    changed = []
    try:
        for container, binary in [(CLIENT, 'wechat-robot-client'), (GATEWAY, 'business-gateway')]:
            changed.append((container, binary))
            replace(container, RELEASE / binary)
        for i in range(12):
            try:
                verify()
                break
            except Exception:
                if i == 11:
                    raise
                time.sleep(2)
        # Source updates are bounded and backed up, never overwrite deployment config.
        with tarfile.open(RELEASE / 'backup/changed-source.tar.gz', 'w:gz') as t:
            for p in (RELEASE / 'payload').rglob('*'):
                if p.is_file():
                    old = BASE / p.relative_to(RELEASE / 'payload')
                    if old.exists():
                        t.add(old, arcname=str(p.relative_to(RELEASE / 'payload')))
        for p in (RELEASE / 'payload').rglob('*'):
            if p.is_file():
                dst = BASE / p.relative_to(RELEASE / 'payload')
                dst.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(p, dst)
        run(['docker', 'tag', 'jiqiren/wechat-robot-client:production-notice-20260927', 'registry.cn-shenzhen.aliyuncs.com/houhou/wechat-robot-client:latest'])
        run(['docker', 'tag', 'jiqiren/business-gateway:production-notice-20260927', 'jiqiren/business-gateway:local'])
        (RELEASE / 'deployed.ok').write_text('ok')
        print('DEPLOYED_DISABLED', flush=True)
    except Exception:
        for container, binary in reversed(changed):
            replace(container, RELEASE / 'backup' / binary)
        print('ROLLED_BACK_BINARIES', flush=True)
        raise

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('step', choices=['prepare', 'build', 'switch', 'verify'])
    globals()[parser.parse_args().step]()
