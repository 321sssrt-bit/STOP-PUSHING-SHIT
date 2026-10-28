# Sanitized historical command. Inspect as text; never execute.
python3 - <<'PY'
from pathlib import Path
import subprocess,io,tarfile
root=Path('fixture-prs')
buf=io.BytesIO()
with tarfile.open(fileobj=buf,mode='w') as t:
 for n in [1,2,3]:t.add(root/f'PR-{n}.md',arcname=f'PR-{n}.md')
r=subprocess.run(['ssh','-o','BatchMode=yes','fixture-host','mkdir -p /tmp/fixture-pr-bodies && tar -xf - -C /tmp/fixture-pr-bodies'],input=buf.getvalue(),capture_output=True);r.check_returncode()
script=r'''
import subprocess,pathlib,json
root=pathlib.Path('/tmp/fixture-pr-bodies')
gh='/opt/tools/gh'
repo='example/project'
branches=['fix/example-one','feat/example-two','feat/example-three']
commits=['revision-one','revision-two','revision-three']
prs=[]
for index,branch in enumerate(branches):
 p=subprocess.run([gh,'api','--method','GET','repos/'+repo+'/pulls','-f','head=fixture-user:'+branch,'-f','state=all'],capture_output=True,text=True)
 if p.returncode:raise RuntimeError(p.stderr)
 existing=json.loads(p.stdout)
 if existing:
  if len(existing)!=1:raise RuntimeError('Multiple matching PRs; refusing duplicate write')
  pr=existing[0]
 else:
  draft=(root/f'PR-{index+1}.md').read_text()
  title,body=draft.split('\n',1);title=title.removeprefix('# ').strip();body=body.lstrip()
  if index:
   dependency=prs[-1]['number']
   comparison='https://github.com/example/project/compare/'+commits[index-1]+'...'+commits[index]
   body=(f'**Stacked PR {index+1}/3; depends on #{dependency}.** This draft targets upstream `main`, so GitHub currently shows the prerequisite changes too. '
    f'[Review this PR\'s own delta]({comparison}). Merge the prerequisites first, then reconcile this branch with upstream before marking it ready.\n\n')+body
  payload={'title':title,'body':body,'base':'main','head':'fixture-user:'+branch,'draft':index>0,'maintainer_can_modify':True}
  (root/f'PR-{index+1}-published.md').write_text('# '+title+'\n\n'+body)
  request=root/f'PR-{index+1}-request.json';request.write_text(json.dumps(payload))
  p=subprocess.run([gh,'api','--method','POST','repos/'+repo+'/pulls','--input',str(request)],capture_output=True,text=True)
  if p.returncode:raise RuntimeError('PR write failed; verify state before retry: '+p.stderr)
  pr=json.loads(p.stdout)
 record={'number':pr['number'],'url':pr['html_url'],'title':pr['title'],'draft':pr['draft'],'head':pr['head']['ref'],'head_sha':pr['head']['sha'],'base':pr['base']['ref'],'state':pr['state']}
 assert record['head_sha']==commits[index]
 prs.append(record)
 (root/'publication.json').write_text(json.dumps(prs,indent=2)+'\n')
 print(json.dumps(record),flush=True)
'''
r=subprocess.run(['ssh','-o','BatchMode=yes','fixture-host','python3 -'],input=script,capture_output=True,text=True)
print(r.stdout);print(r.stderr)
(root/'evidence'/'pr-create-output.txt').write_text(r.stdout+r.stderr)
r.check_returncode()
PY
