# 결정 기록
- 2026-10-26. 사용자가 기존 Linux 설치기를 유지한 DSM 분기 구현·게시를 승인했다. Oracle-A1의 바인딩 질문은 별도 진단 요청이므로 설정은 변경하지 않는다.
- DSM 7.4.1-90080-1, x86_64, 5.10.55+. systemctl·synosystemctl·synouser·synogroup·util-linux su(-g 지원)는 있고 getent/useradd/runuser/systemd-run은 없다.
- Oracle 실측. 127.0.0.1:7438은 HTTP200, 10.0.0.129:7438은 연결 거부, NPM bridge 172.17.0.2에서 같은 upstream도 연결 거부, 공개 HTTPS는502. 사설 IP 바인딩 권장이나 OCI NAT/방화벽에 따라 직접 노출이 가능하므로 포트 접근 제한을 별도로 확인한다.
- Synology CLI는 계정 생성 암호를 인자로 요구한다. 사용자 앱 관리자 암호와 별개인 무작위 서비스 암호라도 잠깐 argv에 노출될 수 있으므로, DSM UI 1회 생성(권장)과 자동 생성 중 선택을 비동기로 요청했다.
- 사용자는 DSM 제어판에서 modudock 계정을 한 번 생성한 뒤 설치하는 방식을 선택했다. synouser로 계정을 자동 생성하거나 기존 DSM 암호를 변경하지 않는다. 전용 그룹만 없는 경우 synogroup으로 추가한다.
- DSM 감지 함수 부재 RED 후 감지·볼륨 경로·unit 생성·su 인자 주입 방지·root/administrator 거부 테스트 GREEN. 기존 Linux 설치 회귀 테스트도 통과했다.
- 공통 설치 잠금의 부모 심볼릭 링크 수용을 RED로 재현했다. /run/modudock-installer의 현재 root 소유 0700 전용 디렉터리로 옮기고 대상 타입·소유권·링크를 확인한다. 잠금 FD9는 비root 명령에 전달하지 않는다. Linux/DSM 전체 테스트가 GREEN이다.
- NAS의 실제 사용자·서비스·방화벽은 변경하지 않았다. Oracle도 진단만 했으며 사용자에게 사설 IP 바인딩 drop-in을 안내했다.
- 독립 리뷰에서 DSM 업데이트 테스트 누락과 su 검사 시점이 늦다는 지적을 받았다. 비활성/암호 변경 요구 계정에서 기존 서비스 중지로 이어지는 효과를 중요 문제로 재분류했다. 실패 su 테스트가 서비스 중지로 RED인 것을 확인하고 DSM에서는 중지 전 사전 검사로 이동했다. DSM 업데이트의 su 잠금·전체 백업·unit 보존도 실제 흐름 테스트에 추가했다.
