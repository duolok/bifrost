from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    database_url: str = "postgresql://bifrost:localdev@localhost:5432/bifrost"
    port: int = 8090
    env: str = "dev"
    model_config = {"env_prefix": "BF_ANALYTICS_"}


settings = Settings()
