# Orbit — Claude Code Integration Guide

> 이 문서를 프로젝트의 `CLAUDE.md`에 포함하면 Claude Code가 배포·모니터링을 자동으로 수행합니다.

---

## Orbit이란?

Vercel, Koyeb, Supabase 등 여러 클라우드 플랫폼에 분산 배포된 서비스를 하나의 CLI로 모니터링하는 도구.
JSON 출력과 exit code 규칙을 통해 Claude Code와의 자동화 워크플로우를 지원한다.

---

## 핵심 워크플로우: Push → Watch → React

```bash
# 1. 코드 변경 후 push
git add -A && git commit -m "fix: resolve payment module" && git push origin main

# 2. 배포 감시 (JSON으로 결과 수신)
orbit watch <project> --service <name> --format json

# 3. exit code에 따라 분기
#   exit 0 → 배포 성공. 사용자에게 완료 보고.
#   exit 1 → 빌드/배포 실패. JSON의 logs를 분석하여 코드 수정 후 재push.
#   exit 2 → 새 배포 미감지. 브랜치 설정 확인 안내.
#   exit 3 → 타임아웃. 대시보드 확인 안내.
```

---

## Exit Code 규칙

| Code | 의미 | Claude Code 대응 |
|------|------|-----------------|
| `0` | 배포 성공, healthy | 완료 보고 |
| `1` | 빌드/배포 실패 | `logs` 필드 파싱 → 에러 원인 분석 → 코드 수정 → 재push |
| `2` | 새 배포 미감지 (60초) | 브랜치/auto-deploy 설정 확인 안내 |
| `3` | 타임아웃 (기본 300초) | 재시도 또는 대시보드 확인 안내 |

---

## 명령어 레퍼런스

### 상태 확인

```bash
# 전체 프로젝트 상태 (빠른 확인)
orbit status

# 특정 프로젝트 상세
orbit status <project>

# 특정 서비스 상세 메트릭
orbit status <project> --service <name>

# JSON 출력 (파싱용)
orbit status --format json
orbit status <project> --format json
```

### 로그 조회

```bash
# 서비스 로그 (최근)
orbit logs <project> --service <name>

# 에러만 필터
orbit logs <project> --service <name> --level error

# 최근 N줄
orbit logs <project> --service <name> --tail 50

# 시간 범위
orbit logs <project> --service <name> --since 30m

# 실시간 스트리밍 (Ctrl+C로 종료)
orbit logs <project> --service <name> --follow
```

### 배포 이력

```bash
# 전체 배포 이력
orbit deploys <project>

# 특정 서비스 배포 이력
orbit deploys <project> --service <name> --limit 20

# JSON 출력
orbit deploys <project> --format json

# 특정 배포 상세
orbit deploy <project> --service <name> --id <deploy-id>
orbit deploy <project> --service <name> --id <deploy-id> --format json
```

### 배포 감시 (Watch)

```bash
# 단일 서비스 watch
orbit watch <project> --service <name>

# JSON 출력 (Claude Code 자동화용 — 항상 이것을 사용)
orbit watch <project> --service <name> --format json

# 타임아웃 설정 (초)
orbit watch <project> --service <name> --timeout 300

# 복수 서비스 동시 감시
orbit watch <project> --service api,frontend --format json

# 전체 서비스 감시
orbit watch <project> --all --format json
```

### 배포 액션

```bash
# 재배포 (현재 코드 그대로)
orbit redeploy <project> --service <name>

# 롤백 (이전 배포로)
orbit rollback <project> --service <name>
orbit rollback <project> --service <name> --to <deploy-id>
```

### 스케일링

```bash
# 현재 스케일 정보
orbit scale <project> --service <name>

# 인스턴스 수 변경 (Scale Out)
orbit scale <project> --service <name> --min 3
orbit scale <project> --service <name> --min 2 --max 8

# 인스턴스 타입 변경 (Scale Up — 재배포 발생, 확인 프롬프트)
orbit scale <project> --service <name> --type small
```

### 프로젝트 관리

```bash
# 프로젝트 목록
orbit projects

# 프로젝트 상세
orbit project <name>

# 프로젝트 생성
orbit project create <name>
orbit project create <name> --auto   # 서비스 자동 감지

# 프로젝트 삭제 (확인 프롬프트)
orbit project delete <name>
```

### 서비스 관리

```bash
# 서비스 추가
orbit service add <project> --name <name> --platform <platform> --id <service-id>

# 서비스 제거
orbit service remove <project> --name <name>
```

### 토폴로지

```bash
# 토폴로지 조회
orbit topology <project>

# 토폴로지 순서 설정
orbit topology <project> --set "frontend → api → db"
orbit topology <project> --set "frontend -> api -> db"   # -> 도 가능
```

### 설정

```bash
# 현재 설정 조회
orbit config

# 기본 프로젝트 설정
orbit config set default-project <name>

# 임계값 설정
orbit config set threshold.response-time 500
orbit config set threshold.cpu 80
orbit config set threshold.memory 85
```

### 플랫폼 연결

```bash
# 플랫폼 연결 (대화형)
orbit connect <platform>

# 비대화형 (CI/CD, 스크립트)
orbit connect <platform> --token "TOKEN"

# 연결 상태 확인
orbit connections

# 연결 해제
orbit disconnect <platform>
```

지원 플랫폼: `vercel`, `koyeb`, `supabase`

### 헬스체크 (Cold Start 방지)

```bash
# 헬스체크 등록
orbit heartbeat <project> --service <name> --url https://api.example.com/health
orbit heartbeat <project> --service <name> --url https://api.example.com/health --interval 5m

# 헬스체크 상태 확인 (등록된 URL 즉시 ping)
orbit heartbeat <project>

# 헬스체크 제거
orbit heartbeat <project> --service <name> --remove
```

---

## JSON 출력 스키마

### `orbit watch --format json`

**성공 (exit 0):**
```json
{
  "result": "success",
  "service": "api",
  "platform": "koyeb",
  "deploy_id": "deploy_xyz",
  "commit": "a1b2c3d",
  "duration_sec": 58,
  "status": "healthy",
  "url": "https://api.example.com"
}
```

**실패 (exit 1) — `logs` 필드로 에러 원인 분석:**
```json
{
  "result": "failed",
  "service": "api",
  "platform": "koyeb",
  "deploy_id": "deploy_xyz",
  "commit": "f8g9h0i",
  "duration_sec": 32,
  "phase": "build",
  "error": "Cannot find module 'stripe'",
  "logs": [
    "npm ERR! Cannot find module 'stripe'",
    "src/payment.ts(3,28): error TS2307: Cannot find module 'stripe'",
    "npm ERR! code ELIFECYCLE"
  ]
}
```

**새 배포 없음 (exit 2):**
```json
{
  "result": "no_deployment",
  "service": "api",
  "platform": "koyeb",
  "current_deploy_id": "deploy_abc",
  "waited_sec": 60,
  "reason": "No new deployment detected"
}
```

**타임아웃 (exit 3):**
```json
{
  "result": "timeout",
  "service": "api",
  "platform": "koyeb",
  "deploy_id": "deploy_xyz",
  "phase": "build",
  "elapsed_sec": 300
}
```

### `orbit status --format json`

```json
{
  "project-name": [
    {
      "name": "api",
      "platform": "koyeb",
      "id": "svc_xxx",
      "status": "healthy",
      "response_ms": 45,
      "cpu": 23.5,
      "memory": 67.2,
      "instances": 2,
      "max_instances": 5,
      "last_deploy": {
        "id": "deploy_abc",
        "status": "healthy",
        "commit": "a1b2c3d",
        "message": "fix: resolve bug",
        "created_at": "2026-02-26T10:30:00Z",
        "url": "https://api.example.com"
      }
    }
  ]
}
```

### `orbit deploys --format json`

```json
[
  {
    "service": "api",
    "platform": "koyeb",
    "deployments": [
      {
        "id": "deploy_abc",
        "status": "healthy",
        "commit": "a1b2c3d",
        "message": "fix: resolve bug",
        "created_at": "2026-02-26T10:30:00Z",
        "duration": "45s",
        "url": "https://api.example.com"
      }
    ]
  }
]
```

---

## 자동화 패턴

### 패턴 1: 배포 후 자동 검증

```bash
git push origin main
RESULT=$(orbit watch myproject --service api --format json)
EXIT=$?

if [ $EXIT -eq 0 ]; then
  echo "배포 완료: $(echo $RESULT | jq -r '.url')"
elif [ $EXIT -eq 1 ]; then
  echo "실패 원인:"
  echo $RESULT | jq -r '.logs[]'
fi
```

### 패턴 2: 빌드 실패 자동 대응

`orbit watch` exit 1이면:

1. JSON의 `logs` 배열을 읽는다
2. 에러 메시지에서 원인을 파악한다 (missing module, type error, syntax error 등)
3. 해당 코드를 수정한다
4. 다시 commit & push한다
5. `orbit watch`로 재감시한다

### 패턴 3: 장애 진단

```bash
# 1. 전체 상태 확인
orbit status --format json

# 2. 문제 서비스 로그 확인
orbit logs myproject --service api --level error --tail 50

# 3. 최근 배포 이력 확인
orbit deploys myproject --service api --limit 5 --format json

# 4. 필요시 롤백
orbit rollback myproject --service api
```

### 패턴 4: 성능 문제 대응

```bash
# 1. 메트릭 확인
orbit status myproject --service api

# 2. 응답시간/CPU 높으면 스케일 아웃
orbit scale myproject --service api --min 3

# 3. 스케일 확인
orbit scale myproject --service api
```

---

## CLAUDE.md에 복사할 내용

아래 블록을 프로젝트의 `CLAUDE.md`에 추가하면 Claude Code가 orbit을 활용합니다:

```markdown
## 배포 모니터링 (Orbit)

이 프로젝트는 `orbit` CLI로 배포를 관리합니다.

- 코드 push 후 반드시 `orbit watch <project> --service <name> --format json`으로 배포를 확인하세요.
- exit 0이면 완료 보고, exit 1이면 JSON의 logs를 분석하여 코드를 수정하고 재push하세요.
- 상태 확인: `orbit status --format json`
- 로그 확인: `orbit logs <project> --service <name> --level error --tail 50`
- 장애 시 롤백: `orbit rollback <project> --service <name>`

프로젝트 이름: <YOUR_PROJECT>
서비스 목록: orbit projects 또는 orbit topology <YOUR_PROJECT> 으로 확인
```
