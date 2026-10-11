#!/usr/bin/env python3
"""Check ClickShield authenticated analytics endpoints without printing secrets."""
import argparse
import http.cookiejar
import json
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base',required=True,help='ClickShield origin, for example https://clickshield.example.com')
    parser.add_argument('--env-file',type=Path,default=Path('.env.production'))
    parser.add_argument('--enable-replay',action='store_true')
    args=parser.parse_args()
    env=dict(line.split('=',1) for line in args.env_file.read_text().splitlines() if '=' in line and not line.startswith('#'))
    base=args.base.rstrip('/')
    opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    csrf=''
    def request(path,body=None,method=None,auth=True,origin=None):
        content_type="application/json" if isinstance(body,dict) else "application/x-www-form-urlencoded"
        if isinstance(body,dict):body=json.dumps(body).encode()
        req=urllib.request.Request(base+path,data=body,method=method)
        if body:req.add_header('Content-Type',content_type)
        if csrf:req.add_header('X-CSRF-Token',csrf)
        if origin:req.add_header('Origin',origin)
        client=opener if auth else urllib.request.build_opener()
        try:
            with client.open(req,timeout=30) as r:
                data=r.read()
                try:data=json.loads(data)
                except (ValueError,UnicodeDecodeError):pass
                return r.status,data
        except urllib.error.HTTPError as e:
            return e.code,e.read()
    assert request('/healthz',auth=False)[0]==200
    status,_=request('/login',urllib.parse.urlencode({'username':env.get('ADMIN_USERNAME','admin'),'password':env['ADMIN_PASSWORD']}).encode(),method='POST',origin=base)
    assert status==200,'Login failed'
    status,me=request('/v1/me');assert status==200;csrf=me['csrf_token']
    status,data=request('/v1/projects');assert status==200
    project=next(p for p in data['projects'] if p['public_key']==env['BOOTSTRAP_PROJECT_KEY'])
    pid=project['id']
    for kind in ['sources','referrers','campaigns','utm','terms','landing','exit','devices','browsers','geo','languages','networks','quality','bots','daily','hours','activity']:
        status,report=request('/v1/reports?project_id='+pid+'&report='+kind)
        assert status==200,kind
        # Rows form a hierarchy; top-level rows partition the visits.
        assert report['kind']==kind and sum(row['visits'] for row in report['rows'] if row['level']==1)==report['totals']['visits'],kind
    period='&from='+urllib.parse.quote('2026-01-01T00:00:00Z')+'&to='+urllib.parse.quote('2026-02-01T00:00:00Z')
    status,timeline=request('/v1/reports/timeline?project_id='+pid+'&report=sources&interval=day&timezone=UTC'+period)
    assert status==200 and len(timeline['buckets'])==31 and len(timeline['total'])==31,'timeline'
    for kind in ['events','goals','funnels','journey','heatmap','retention']:
        assert request('/v1/analytics/'+kind+'?project_id='+pid)[0]==200,kind
    assert request('/analytics')[0]==200
    assert request('/v1/reports',auth=False)[0]==401
    assert request('/v1/reports?report=invalid')[0]==400
    assert request('/v1/reports?report=daily&timezone=invalid')[0]==400
    for path in ['/v1/traffic?project_id='+pid,'/v1/stats?project_id='+pid,'/v1/sessions?project_id='+pid,'/v1/projects/'+pid+'/metrika-read']:
        assert request(path)[0]==200,path
    assert request('/v1/traffic',auth=False)[0]==401
    assert request('/v1/sessions/12345678-1234-4123-8123-123456789abc/replay',auth=False)[0]==401
    assert request('/v1/replay?key='+urllib.parse.quote(project['public_key']),{},method='POST',auth=False,origin='https://not-allowed.example')[0]==403
    if args.enable_replay:
        assert request('/v1/projects/'+pid+'/replay',{'enabled':True,'sample_rate':100,'retention_days':7},'PUT')[0]==200
    status,cfg=request('/v1/tracker/config?key='+urllib.parse.quote(project['public_key']),auth=False,origin=project['allowed_origins'][0]);assert status==200
    if args.enable_replay:assert cfg['replay_enabled']
    for name in ['rrweb-record.js','rrweb-replay.js','replay-recorder.js','replay-viewer.js','replay.html','analytics.js','product-analytics.js','analytics-tools.js','reports.css']:
        assert request('/assets/'+name,auth=False)[0]==200,name
    print(json.dumps({'health':'ok','authenticated_analytics':'ok','unauthorized_access':'rejected','foreign_origin':'rejected','replay_enabled':cfg['replay_enabled'],'project_id':pid},ensure_ascii=False))
if __name__=='__main__':main()
