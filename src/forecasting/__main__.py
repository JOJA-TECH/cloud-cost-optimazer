"""Punto de entrada del módulo forecasting.

Uso:
    python -m forecasting                 # CLI de entrenamiento (mismo que forecasting.cli)
    python -m forecasting.serving.server # servidor gRPC de inferencia online
"""

from forecasting.cli import main

if __name__ == "__main__":
    raise SystemExit(main())
