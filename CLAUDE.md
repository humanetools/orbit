# Orbit — Rules for Claude Code

## 반드시 지켜야 할 규칙

### 릴리스
- **로컬에서 바이너리 빌드/릴리스 금지.** GitHub Actions + GoReleaser가 태그 push 시 자동으로 크로스 컴파일 및 릴리스를 생성한다 (`.github/workflows/release.yml`).
- 릴리스 절차: `git tag -a vX.Y.Z -m "메시지"` → `git push origin vX.Y.Z` → 끝. 수동 `gh release create` 하지 마라.
- `gh release create`로 먼저 릴리스를 만들면 Actions가 중복 충돌로 실패한다.

### 빌드
- Go 경로: `export PATH=~/go-sdk/go/bin:~/go/bin:$PATH`
- 빌드: `make build` (ldflags로 버전 주입됨)
- 검증: `go build ./... && go vet ./...`

### 커밋
- **`Co-Authored-By` 트레일러를 커밋 메시지에 절대 포함하지 마라.** GitHub contributors에 Claude가 나타나는 원인이 된다.
- docs/ 폴더는 `.gitignore`에 포함되어 있다. 공개 문서는 프로젝트 루트에 배치한다.

---

## 작업 기록 규칙

**중요: 자동 기록 금지** — 일지/트러블슈팅을 자동으로 작성하지 말 것. 사용자가 기록하라고 지시할 때만 작성.

### 작업 일지 (사용자 지시 시)
- 사용자가 기록을 요청하면 `docs/daily/YYMMDD.md`에 기록
- 단순 질의응답, 조회만 한 경우는 기록 불필요
- 형식:
```markdown
## HH:MM | 작업 제목
- **분류**: 기능개발 | 버그수정 | 리팩터링 | 문서 | 인프라 | 설정
- **요약**: 무엇을 왜 했는지 1~2줄
- **수정 파일**:
  - `path/to/file` (신규 | 수정 | 삭제 | 이동)
- **검증**: go build 통과 / go vet 통과 / 테스트 통과 / 해당 없음
- **트러블슈팅**: 없음 | `docs/troubleshooting/YYMMDD-xxx.md`에 기록
```

### 트러블슈팅 기록 (사용자 지시 시)
- 사용자가 기록을 요청하면 작성
- 기존 파일에 추가하거나 새 파일 생성: `docs/troubleshooting/YYMMDD-{주제}.md`
- 형식: 문제 → 시도 → 해결 → 교훈

### 작업 전 참조
- 코드 수정 전 관련 `docs/troubleshooting/*.md` 파일을 읽고 시작
- 프로젝트 내 유사 패턴이 있으면 먼저 읽고 따를 것
