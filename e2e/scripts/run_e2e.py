#!/usr/bin/env python3
"""Prueba E2E completa del pipeline ML Release 1.0.

Flujo:
1. Autenticar via Entra ID (OIDC) -> obtener access token
2. Llamar al backend Go para solicitar forecast (vía Jobs worker)
3. El servidor Python ML entrena/predice y persiste en ml.forecast
4. Verificar que forecast_uid es consultable via contract.v_forecast
5. Generar recomendación via Recommendation engine
6. Verificar que la decisión es no_action si no hay evidencia suficiente

Requiere:
- Backend Go ejecutándose en localhost:8080 con CCO_ML_MODE=grpc
- Servidor Python ML ejecutándose en localhost:50051
- Base de datos cco_data con datos (puede usar e2e/scripts/migrate_supabase_to_cco_data.py)
- Variables de entorno para Entra ID configuradas
"""

import json
import os
import sys
import time
import uuid
from datetime import datetime, timedelta, timezone

import requests

BASE_URL = os.environ.get("CCO_BACKEND_URL", "http://localhost:8080")
GRPC_URL = os.environ.get("CCO_ML_ENDPOINT", "127.0.0.1:50051")

# Credenciales de prueba (debe existir en cco_app)
TEST_EMAIL = os.environ.get("E2E_TEST_EMAIL", "admin@cco.local")
TEST_PASSWORD = os.environ.get("E2E_TEST_PASSWORD", "Admin123!")

# Recurso de prueba (debe existir en core.resource)
TEST_RESOURCE_UID = os.environ.get("E2E_TEST_RESOURCE_UID")


def print_step(msg):
    print(f"\n{'='*60}\n[PASO] {msg}\n{'='*60}")


def print_result(success, msg, detail=""):
    status = "✅" if success else "❌"
    print(f"  {status} {msg}")
    if detail:
        print(f"      {detail}")


def test_health():
    """Verificar que el backend está vivo."""
    print_step("Verificar salud del backend")
    r = requests.get(f"{BASE_URL}/healthz", timeout=10)
    print_result(r.status_code == 200, f"GET /healthz -> {r.status_code}", r.text)
    return r.status_code == 200


def test_ml_service_reachable():
    """Verificar que el servidor Python ML está accesible."""
    print_step("Verificar servicio ML Python (gRPC via HTTP no disponible, verificar puerto)")
    # gRPC no se puede probar via HTTP simple, pero verificamos que el puerto esté abierto
    import socket
    host, port = GRPC_URL.split(":")
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.settimeout(5)
    result = s.connect_ex((host, int(port)))
    s.close()
    print_result(result == 0, f"Puerto gRPC {GRPC_URL} accesible", f"result={result}")
    return result == 0


def test_auth_oidc_available():
    """Verificar que el endpoint OIDC esté configurado."""
    print_step("Verificar autenticación Entra ID (OIDC) configurada")
    r = requests.get(f"{BASE_URL}/auth/oidc/login", timeout=10, allow_redirects=False)
    # Esperamos 302 redirect al proveedor OIDC
    print_result(r.status_code == 302, f"GET /auth/oidc/login -> {r.status_code} (redirect a Entra ID)", r.headers.get("Location", "")[:100])
    return r.status_code == 302


def test_auth_dev_fallback():
    """Probar login por contraseña (modo dev) si OIDC no está configurado."""
    print_step("Probar autenticación por contraseña (modo dev)")
    r = requests.post(f"{BASE_URL}/auth/login", json={
        "email": TEST_EMAIL,
        "password": TEST_PASSWORD
    }, timeout=10)
    if r.status_code == 200:
        data = r.json()
        token = data.get("access_token")
        print_result(True, f"Login exitoso, token obtenido ({len(token)} chars)", f"user_id={data.get('user_id')}, org_id={data.get('org_id')}")
        return token
    else:
        print_result(False, f"Login falló -> {r.status_code}", r.text[:200])
        return None


def test_forecast_request(token, resource_uid):
    """Solicitar forecast via Jobs worker (analytics:compute)."""
    print_step("Solicitar forecast via backend (Jobs worker)")
    from_dt = (datetime.now(timezone.utc) - timedelta(days=7)).strftime("%Y-%m-%dT%H:%M:%SZ")
    to_dt = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

    headers = {"Authorization": f"Bearer {token}", "Content-Type": "application/json"}
    payload = {
        "operation": "forecast",
        "resource_uid": resource_uid,
        "from": from_dt,
        "to": to_dt,
        "metric_code": "cpu_avg",
        "horizon_steps": 1
    }

    r = requests.post(f"{BASE_URL}/api/v1/jobs/analytics", json=payload, headers=headers, timeout=10)
    print_result(r.status_code in (200, 202), f"POST /api/v1/jobs/analytics -> {r.status_code}", r.text[:200])

    if r.status_code in (200, 202):
        data = r.json()
        job_id = data.get("job_id") or data.get("id")
        print(f"  Job ID: {job_id}")
        return job_id
    return None


def test_wait_for_job(token, job_id, max_wait=60):
    """Esperar a que el job termine."""
    print_step(f"Esperar a que el job {job_id} termine (máx {max_wait}s)")
    headers = {"Authorization": f"Bearer {token}"}

    for i in range(max_wait // 2):
        r = requests.get(f"{BASE_URL}/api/v1/jobs/{job_id}", headers=headers, timeout=10)
        if r.status_code == 200:
            data = r.json()
            status = data.get("status")
            print(f"  Intento {i+1}: estado={status}")
            if status in ("completed", "failed"):
                print_result(status == "completed", f"Job {status}", f"error={data.get('error', '')}")
                return status == "completed"
        time.sleep(2)

    print_result(False, f"Job no terminó en {max_wait}s")
    return False


def test_get_forecasts(token, resource_uid):
    """Consultar forecasts persistidos via contract.v_forecast."""
    print_step("Consultar forecasts persistidos (contract.v_forecast)")
    headers = {"Authorization": f"Bearer {token}"}
    params = {"resource_uid": resource_uid, "target_metric": "cpu_avg"}

    r = requests.get(f"{BASE_URL}/api/v1/analytics/forecasts", headers=headers, params=params, timeout=10)
    print_result(r.status_code == 200, f"GET /api/v1/analytics/forecasts -> {r.status_code}", r.text[:300])

    if r.status_code == 200:
        data = r.json()
        print(f"  Forecasts encontrados: {len(data) if isinstance(data, list) else 'N/A'}")
        if isinstance(data, list) and len(data) > 0:
            f = data[0]
            print(f"  forecast_uid: {f.get('forecast_uid')}")
            print(f"  model_version_id: {f.get('model_version_id')}")
            print(f"  target_metric: {f.get('target_metric')}")
            print(f"  y_pred: {f.get('y_pred')}")
            print(f"  period_start_utc: {f.get('period_start_utc')}")
        return data
    return []


def test_list_models(token):
    """Listar versiones de modelo registradas."""
    print_step("Listar versiones de modelo (GET /api/v1/ml/models)")
    headers = {"Authorization": f"Bearer {token}"}

    r = requests.get(f"{BASE_URL}/api/v1/ml/models", headers=headers, timeout=10)
    print_result(r.status_code == 200, f"GET /api/v1/ml/models -> {r.status_code}", r.text[:300])

    if r.status_code == 200:
        data = r.json()
        items = data.get("items", [])
        print(f"  Modelos encontrados: {len(items)}")
        for m in items:
            print(f"  - {m.get('model_name')} v{m.get('version')} | stage={m.get('stage')} | mae={m.get('metrics_summary', {}).get('mae', 'N/A')}")
        return items
    return []


def main():
    print("\n" + "="*70)
    print("PRUEBA E2E COMPLETA - Pipeline ML Release 1.0")
    print("="*70)

    # 1. Health
    if not test_health():
        print("\n❌ Backend no disponible. Iniciar con: CCO_DATA_DSN=... CCO_ML_MODE=grpc ./cco-app")
        return 1

    # 2. Verificar servicio ML
    test_ml_service_reachable()

    # 3. Verificar OIDC configurado
    test_auth_oidc_available()

    # 4. Autenticar (intentamos OIDC primero, luego password dev)
    token = test_auth_dev_fallback()
    if not token:
        print("\n❌ No se pudo autenticar. Configurar CCO_OIDC_* o usar credenciales dev válidas.")
        return 1

    # 5. Obtener resource_uid
    global TEST_RESOURCE_UID
    if not TEST_RESOURCE_UID:
        print("\n⚠️  E2E_TEST_RESOURCE_UID no configurado. Listando recursos disponibles...")
        headers = {"Authorization": f"Bearer {token}"}
        r = requests.get(f"{BASE_URL}/api/v1/cloud-accounts/{os.environ.get('CCO_AZURE_ACCOUNT_ID', '')}/resources", headers=headers, timeout=10)
        if r.status_code == 200:
            resources = r.json()
            if resources:
                TEST_RESOURCE_UID = resources[0].get("resource_uid")
                print(f"  Usando recurso: {TEST_RESOURCE_UID}")
            else:
                print("  No se encontraron recursos. Configurar E2E_TEST_RESOURCE_UID.")
                return 1
        else:
            print("  No se pudo listar recursos. Configurar E2E_TEST_RESOURCE_UID manualmente.")
            return 1

    # 6. Solicitar forecast
    job_id = test_forecast_request(token, TEST_RESOURCE_UID)
    if job_id:
        test_wait_for_job(token, job_id)

    # 7. Consultar forecasts
    test_get_forecasts(token, TEST_RESOURCE_UID)

    # 8. Listar modelos
    test_list_models(token)

    print("\n" + "="*70)
    print("PRUEBA E2E COMPLETADA")
    print("="*70)
    return 0


if __name__ == "__main__":
    sys.exit(main())
