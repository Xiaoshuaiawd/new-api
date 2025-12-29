import json, time, hmac, hashlib, base64, os, asyncio, uuid, ssl, re, math, random
from datetime import datetime, timezone, timedelta
from typing import List, Optional, Union, Dict, Any
from dataclasses import dataclass
import logging
from dotenv import load_dotenv

import httpx
from fastapi import FastAPI, HTTPException, Header, Request, Body, Form
from fastapi.responses import StreamingResponse, HTMLResponse, JSONResponse, RedirectResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel
from util.streaming_parser import parse_json_array_stream_async
from collections import deque
from threading import Lock
from functools import wraps

# 导入认证装饰器
from core.auth import (
    ADMIN_SESSION_COOKIE_NAME,
    create_admin_session_cookie,
    require_path_prefix,
    require_admin_auth,
    require_path_and_admin,
    verify_admin_session_cookie,
)

# ---------- 日志配置 ----------

# 内存日志缓冲区 (保留最近 3000 条日志，重启后清空)
# 性能优化：使用线程安全的 deque，减少锁使用
log_buffer = deque(maxlen=3000)
log_lock = Lock()

# 统计数据持久化
STATS_FILE = "stats.json"
stats_lock = Lock()

# 性能优化：批量日志缓冲区，减少锁竞争
_log_batch_buffer = []
_log_batch_size = 50  # 每 50 条日志批量写入一次

def load_stats():
    """加载统计数据"""
    try:
        if os.path.exists(STATS_FILE):
            with open(STATS_FILE, 'r', encoding='utf-8') as f:
                return json.load(f)
    except Exception:
        pass
    return {
        "total_visitors": 0,
        "total_requests": 0,
        "request_timestamps": [],  # 最近1小时的请求时间戳
        "visitor_ips": {}  # {ip: timestamp} 记录访问IP和时间
    }

def save_stats(stats):
    """保存统计数据"""
    try:
        with open(STATS_FILE, 'w', encoding='utf-8') as f:
            json.dump(stats, f, ensure_ascii=False, indent=2)
    except Exception as e:
        logger.error(f"[STATS] 保存统计数据失败: {str(e)[:50]}")

# 初始化统计数据
global_stats = load_stats()

class MemoryLogHandler(logging.Handler):
    """自定义日志处理器，将日志写入内存缓冲区（批量优化）"""
    def emit(self, record):
        global _log_batch_buffer
        log_entry = self.format(record)
        # 转换为北京时间（UTC+8）
        beijing_tz = timezone(timedelta(hours=8))
        beijing_time = datetime.fromtimestamp(record.created, tz=beijing_tz)

        log_item = {
            "time": beijing_time.strftime("%Y-%m-%d %H:%M:%S"),
            "level": record.levelname,
            "message": record.getMessage()
        }

        # 批量写入优化：先缓存到本地列表，减少锁竞争
        _log_batch_buffer.append(log_item)

        # 当缓冲区达到阈值时，批量写入
        if len(_log_batch_buffer) >= _log_batch_size:
            with log_lock:
                log_buffer.extend(_log_batch_buffer)
                _log_batch_buffer.clear()

# 配置日志
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s | %(levelname)s | %(message)s",
    datefmt="%H:%M:%S",
)
logger = logging.getLogger("gemini")

# 添加内存日志处理器
memory_handler = MemoryLogHandler()
memory_handler.setFormatter(logging.Formatter("%(asctime)s | %(levelname)s | %(message)s", datefmt="%H:%M:%S"))
logger.addHandler(memory_handler)

load_dotenv()
# ---------- 配置 ----------
PROXY        = os.getenv("PROXY") or None
TIMEOUT_SECONDS = 600
API_KEY      = os.getenv("API_KEY") or None  # API 访问密钥（可选）
PATH_PREFIX  = os.getenv("PATH_PREFIX")      # 路径前缀（必需，用于隐藏端点）
ADMIN_KEY    = os.getenv("ADMIN_KEY")        # 管理员密钥（必需，用于访问管理端点）
BASE_URL     = os.getenv("BASE_URL")         # 服务器完整URL（可选，用于图片URL生成）

# ---------- 账户存储配置 ----------
# 默认使用 accounts.json / ACCOUNTS_CONFIG；当账号量非常大时可切换到 Redis（避免加载超大 JSON）
ACCOUNTS_SOURCE = os.getenv("ACCOUNTS_SOURCE", "file").strip().lower()  # file|redis
REDIS_URL = os.getenv("REDIS_URL") or None
REDIS_KEY_PREFIX = os.getenv("REDIS_KEY_PREFIX", "gb2api").strip()
REDIS_ACCOUNTS_PRELOAD = int(os.getenv("REDIS_ACCOUNTS_PRELOAD", "200"))  # Redis 模式预加载到内存的账号数（用于加速首请求/管理面板预览）
SHUFFLE_ACCOUNTS_ON_START = os.getenv("SHUFFLE_ACCOUNTS_ON_START", "true").strip().lower() not in {"0", "false", "no"}  # 是否在启动时打乱账号列表（默认true）

# ---------- 公开展示配置 ----------
LOGO_URL     = os.getenv("LOGO_URL", "")  # Logo URL（公开，为空则不显示）
CHAT_URL     = os.getenv("CHAT_URL", "")  # 开始对话链接（公开，为空则不显示）
MODEL_NAME   = os.getenv("MODEL_NAME", "gemini-business")  # 模型名称（公开）
HIDE_HOME_PAGE = os.getenv("HIDE_HOME_PAGE", "").lower() == "true"  # 是否隐藏首页（默认不隐藏）

# ---------- 图片存储配置 ----------
# 自动检测存储路径：优先使用持久化存储，否则使用临时存储
if os.path.exists("/data"):
    IMAGE_DIR = "/data/images"  # HF Pro持久化存储（重启不丢失）
else:
    IMAGE_DIR = "./images"  # 临时存储（重启会丢失）

# ---------- 重试配置 ----------
MAX_NEW_SESSION_TRIES = int(os.getenv("MAX_NEW_SESSION_TRIES", "5"))  # 新会话创建最多尝试账户数（默认5）
MAX_REQUEST_RETRIES = int(os.getenv("MAX_REQUEST_RETRIES", "3"))      # 请求失败最多重试次数（默认3）
MAX_ACCOUNT_SWITCH_TRIES = int(os.getenv("MAX_ACCOUNT_SWITCH_TRIES", "5"))  # 每次重试找账户的最大尝试次数（默认5）
ACCOUNT_FAILURE_THRESHOLD = int(os.getenv("ACCOUNT_FAILURE_THRESHOLD", "3"))  # 账户连续失败阈值（默认3次）
ACCOUNT_COOLDOWN_SECONDS = int(os.getenv("ACCOUNT_COOLDOWN_SECONDS", "300"))  # 账户冷却时间（默认300秒=5分钟）
SESSION_CACHE_TTL_SECONDS = int(os.getenv("SESSION_CACHE_TTL_SECONDS", "3600"))  # 会话缓存过期时间（默认3600秒=1小时）
ENABLE_SESSION_REUSE = os.getenv("ENABLE_SESSION_REUSE", "true").strip().lower() not in {"0", "false", "no"}  # 是否复用会话（默认true）

# ---------- OpenAI usage 字段配置 ----------
# 上游接口不返回 token usage，这里只能做“估算”。如需关闭估算可设置 false/0/no。
ENABLE_USAGE_ESTIMATE = os.getenv("ENABLE_USAGE_ESTIMATE", "true").strip().lower() not in {"0", "false", "no"}

# ---------- Admin 登录配置 ----------
ADMIN_SESSION_TTL_SECONDS = int(os.getenv("ADMIN_SESSION_TTL_SECONDS", "86400"))  # 24h

# ---------- 模型映射配置 ----------
MODEL_MAPPING = {
    "gemini-auto": None,
    "gemini-2.5-flash": "gemini-2.5-flash",
    "gemini-2.5-pro": "gemini-2.5-pro",
    "gemini-3-flash-preview": "gemini-3-flash-preview",
    "gemini-3-pro-preview": "gemini-3-pro-preview"
}

# ---------- HTTP 客户端 ----------
# 高并发优化：大幅提升连接池限制以支持 2w+ 并发
# max_connections: 总连接数上限（建议设置为目标并发的 1.5-2 倍）
# max_keepalive_connections: 保持活跃的连接数（减少频繁建连开销）
http_client = httpx.AsyncClient(
    proxies=PROXY,
    verify=False,
    http2=False,
    timeout=httpx.Timeout(TIMEOUT_SECONDS, connect=60.0),
    limits=httpx.Limits(
        max_keepalive_connections=5000,  # 保持 5000 个活跃连接
        max_connections=30000,            # 最大 30000 个连接（支持 2w+ 并发）
        keepalive_expiry=30.0             # 空闲连接 30 秒后回收
    )
)

# ---------- 工具函数 ----------
def get_base_url(request: Request) -> str:
    """获取完整的base URL（优先环境变量，否则从请求自动获取）"""
    # 优先使用环境变量
    if BASE_URL:
        return BASE_URL.rstrip("/")

    # 自动从请求获取（兼容反向代理）
    forwarded_proto = request.headers.get("x-forwarded-proto", request.url.scheme)
    forwarded_host = request.headers.get("x-forwarded-host", request.headers.get("host"))

    return f"{forwarded_proto}://{forwarded_host}"

# ---------- 常量定义 ----------
USER_AGENT = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"

def get_common_headers(jwt: str) -> dict:
    return {
        "accept": "*/*",
        "accept-encoding": "gzip, deflate, br, zstd",
        "accept-language": "zh-CN,zh;q=0.9,en;q=0.8",
        "authorization": f"Bearer {jwt}",
        "content-type": "application/json",
        "origin": "https://business.gemini.google",
        "referer": "https://business.gemini.google/",
        "user-agent": USER_AGENT,
        "x-server-timeout": "1800",
        "sec-ch-ua": '"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"',
        "sec-ch-ua-mobile": "?0",
        "sec-ch-ua-platform": '"Windows"',
        "sec-fetch-dest": "empty",
        "sec-fetch-mode": "cors",
        "sec-fetch-site": "cross-site",
    }

def urlsafe_b64encode(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).decode().rstrip("=")

def kq_encode(s: str) -> str:
    b = bytearray()
    for ch in s:
        v = ord(ch)
        if v > 255:
            b.append(v & 255)
            b.append(v >> 8)
        else:
            b.append(v)
    return urlsafe_b64encode(bytes(b))

def create_jwt(key_bytes: bytes, key_id: str, csesidx: str) -> str:
    now = int(time.time())
    header = {"alg": "HS256", "typ": "JWT", "kid": key_id}
    payload = {
        "iss": "https://business.gemini.google",
        "aud": "https://biz-discoveryengine.googleapis.com",
        "sub": f"csesidx/{csesidx}",
        "iat": now,
        "exp": now + 300,
        "nbf": now,
    }
    header_b64  = kq_encode(json.dumps(header, separators=(",", ":")))
    payload_b64 = kq_encode(json.dumps(payload, separators=(",", ":")))
    message     = f"{header_b64}.{payload_b64}"
    sig         = hmac.new(key_bytes, message.encode(), hashlib.sha256).digest()
    return f"{message}.{urlsafe_b64encode(sig)}"

# ---------- 多账户支持 ----------
@dataclass
class AccountConfig:
    """单个账户配置"""
    account_id: str
    secure_c_ses: str
    host_c_oses: Optional[str]
    csesidx: str
    config_id: str
    expires_at: Optional[str] = None  # 账户过期时间 (格式: "2025-12-23 10:59:21")

    def get_remaining_hours(self) -> Optional[float]:
        """计算账户剩余小时数"""
        if not self.expires_at:
            return None
        try:
            # 解析过期时间（假设为北京时间）
            beijing_tz = timezone(timedelta(hours=8))
            expire_time = datetime.strptime(self.expires_at, "%Y-%m-%d %H:%M:%S")
            expire_time = expire_time.replace(tzinfo=beijing_tz)

            # 当前时间（北京时间）
            now = datetime.now(beijing_tz)

            # 计算剩余时间
            remaining = (expire_time - now).total_seconds() / 3600
            return remaining
        except Exception:
            return None

    def is_expired(self) -> bool:
        """检查账户是否已过期"""
        remaining = self.get_remaining_hours()
        if remaining is None:
            return False  # 未设置过期时间，默认不过期
        return remaining <= 0

def format_account_expiration(remaining_hours: Optional[float]) -> tuple:
    """
    格式化账户过期时间显示（基于12小时过期周期）

    Args:
        remaining_hours: 剩余小时数（None表示未设置过期时间）

    Returns:
        (status, status_color, expire_display) 元组
    """
    if remaining_hours is None:
        # 未设置过期时间时显示为"未设置"
        return ("未设置", "#9e9e9e", "未设置")
    elif remaining_hours <= 0:
        return ("已过期", "#f44336", "已过期")
    elif remaining_hours < 3:  # 少于3小时
        return ("即将过期", "#ff9800", f"{remaining_hours:.1f} 小时")
    else:  # 3小时及以上，统一显示小时
        return ("正常", "#4caf50", f"{remaining_hours:.1f} 小时")

class AccountManager:
    """单个账户管理器"""
    def __init__(self, config: AccountConfig):
        self.config = config
        self.jwt_manager: Optional['JWTManager'] = None  # 延迟初始化
        self.is_available = True
        self.last_error_time = 0.0
        self.error_count = 0
        # 429 专用冷却机制
        self.rate_limit_until = 0.0  # 429 限流解除时间戳
        self.rate_limit_cooldown = 18000  # 5 小时 = 18000 秒

    async def get_jwt(self, request_id: str = "") -> str:
        """获取 JWT token (带错误处理)"""
        try:
            if self.jwt_manager is None:
                # 延迟初始化 JWTManager (避免循环依赖)
                self.jwt_manager = JWTManager(self.config)
            jwt = await self.jwt_manager.get(request_id)
            self.is_available = True
            self.error_count = 0
            return jwt
        except Exception as e:
            self.last_error_time = time.time()
            self.error_count += 1
            # 使用配置的失败阈值
            if self.error_count >= ACCOUNT_FAILURE_THRESHOLD:
                self.is_available = False

                # 写入 Redis（多实例共享禁用状态）
                disabled_until = time.time() + ACCOUNT_COOLDOWN_SECONDS
                redis_set_account_disabled(
                    self.config.account_id,
                    disabled_until,
                    f"JWT获取连续失败{self.error_count}次"
                )

                logger.error(f"[ACCOUNT] [{self.config.account_id}] JWT获取连续失败{self.error_count}次，账户已标记为不可用")
            else:
                # 安全：只记录异常类型，不记录详细信息
                logger.warning(f"[ACCOUNT] [{self.config.account_id}] JWT获取失败({self.error_count}/{ACCOUNT_FAILURE_THRESHOLD}): {type(e).__name__}")
            raise

    def should_retry(self) -> bool:
        """检查账户是否可重试（优先从 Redis 读取禁用状态，支持多实例共享）"""
        current_time = time.time()

        # 优先从 Redis 读取禁用状态（多实例共享）
        if REDIS_URL:
            disabled_info = redis_get_account_disabled(self.config.account_id)
            if disabled_info:
                disabled_until = disabled_info.get("disabled_until", 0)
                if current_time < disabled_until:
                    # 仍在禁用期内
                    return False
                else:
                    # 禁用期已过，清除 Redis 状态并解锁
                    redis_clear_account_disabled(self.config.account_id)
                    self.is_available = True
                    self.error_count = 0
                    reason = disabled_info.get("reason", "未知")
                    logger.info(f"[ACCOUNT] [{self.config.account_id}] 禁用期已过（原因: {reason}），账户已自动解锁")
                    return True

        # 检查普通失败冷却
        if self.is_available:
            return True
        return time.time() - self.last_error_time > ACCOUNT_COOLDOWN_SECONDS

class MultiAccountManager:
    """多账户协调器"""
    def __init__(self):
        self.accounts: Dict[str, AccountManager] = {}
        self.account_list: List[str] = []  # 账户ID列表 (用于轮询)
        self.current_index = 0
        # 性能优化：使用细粒度锁，减少锁竞争
        self._account_lock = asyncio.Lock()  # 仅用于账户选择
        self._cache_lock = asyncio.Lock()    # 仅用于缓存操作
        # 全局会话缓存：{conv_key: {"account_id": str, "session_id": str, "updated_at": float}}
        self.global_session_cache: Dict[str, dict] = {}
        self.cache_max_size = 1000  # 最大缓存条目数
        self.cache_ttl = SESSION_CACHE_TTL_SECONDS  # 缓存过期时间（秒）

    def _clean_expired_cache(self):
        """清理过期的缓存条目"""
        current_time = time.time()
        expired_keys = [
            key for key, value in self.global_session_cache.items()
            if current_time - value["updated_at"] > self.cache_ttl
        ]
        for key in expired_keys:
            del self.global_session_cache[key]
        if expired_keys:
            logger.info(f"[CACHE] 清理 {len(expired_keys)} 个过期会话缓存")

    def _ensure_cache_size(self):
        """确保缓存不超过最大大小（LRU策略）"""
        if len(self.global_session_cache) > self.cache_max_size:
            # 按更新时间排序，删除最旧的20%
            sorted_items = sorted(
                self.global_session_cache.items(),
                key=lambda x: x[1]["updated_at"]
            )
            remove_count = len(sorted_items) - int(self.cache_max_size * 0.8)
            for key, _ in sorted_items[:remove_count]:
                del self.global_session_cache[key]
            logger.info(f"[CACHE] LRU清理 {remove_count} 个最旧会话缓存")

    async def start_background_cleanup(self):
        """启动后台缓存清理任务（每5分钟执行一次）"""
        try:
            while True:
                await asyncio.sleep(300)  # 5分钟
                async with self._cache_lock:
                    self._clean_expired_cache()
                    self._ensure_cache_size()
        except asyncio.CancelledError:
            logger.info("[CACHE] 后台清理任务已停止")
        except Exception as e:
            logger.error(f"[CACHE] 后台清理任务异常: {e}")

    async def set_session_cache(self, conv_key: str, account_id: str, session_id: str):
        """线程安全地设置会话缓存"""
        async with self._cache_lock:
            self.global_session_cache[conv_key] = {
                "account_id": account_id,
                "session_id": session_id,
                "updated_at": time.time()
            }
            # 检查缓存大小
            self._ensure_cache_size()

    async def update_session_time(self, conv_key: str):
        """线程安全地更新会话时间戳"""
        async with self._cache_lock:
            if conv_key in self.global_session_cache:
                self.global_session_cache[conv_key]["updated_at"] = time.time()

    def add_account(self, config: AccountConfig):
        """添加账户"""
        manager = AccountManager(config)
        self.accounts[config.account_id] = manager
        if config.account_id not in self.account_list:
            self.account_list.append(config.account_id)
        logger.info(f"[MULTI] [ACCOUNT] 添加账户: {config.account_id}")

    async def _ensure_account_loaded(self, account_id: str, request_id: str = "") -> AccountManager:
        """Redis 模式：按需加载单个账户配置并创建 AccountManager。"""
        if account_id in self.accounts:
            return self.accounts[account_id]

        if not getattr(self, "_redis_enabled", False):
            raise HTTPException(404, f"Account {account_id} not found")

        req_tag = f"[req_{request_id}] " if request_id else ""
        cfg = await asyncio.to_thread(redis_get_account_config, account_id)
        if not cfg:
            # 账号在 Redis 中不存在，剔除
            try:
                self.account_list.remove(account_id)
            except ValueError:
                pass
            logger.warning(f"[MULTI] [ACCOUNT] {req_tag}Redis 未找到账户配置: {account_id}")
            raise HTTPException(404, f"Account {account_id} not found")

        required_fields = ["secure_c_ses", "csesidx", "config_id"]
        missing_fields = [f for f in required_fields if f not in cfg]
        if missing_fields:
            logger.warning(f"[MULTI] [ACCOUNT] {req_tag}账户 {account_id} 缺少字段: {missing_fields}")
            raise HTTPException(400, f"Account {account_id} invalid config")

        # 过期检查
        config = AccountConfig(
            account_id=cfg.get("id") or account_id,
            secure_c_ses=cfg["secure_c_ses"],
            host_c_oses=cfg.get("host_c_oses"),
            csesidx=cfg["csesidx"],
            config_id=cfg["config_id"],
            expires_at=cfg.get("expires_at")
        )
        if config.is_expired():
            # 过期则从列表剔除，避免反复命中
            try:
                self.account_list.remove(account_id)
            except ValueError:
                pass
            logger.warning(f"[MULTI] [ACCOUNT] {req_tag}账户 {account_id} 已过期，已剔除")
            raise HTTPException(503, f"Account {account_id} expired")

        manager = AccountManager(config)
        self.accounts[account_id] = manager
        return manager

    async def get_account(self, account_id: Optional[str] = None, request_id: str = "") -> AccountManager:
        """获取账户 (轮询或指定)"""
        async with self._account_lock:
            req_tag = f"[req_{request_id}] " if request_id else ""

            # Redis 模式：允许空启动，首次请求时若列表仍为空则尝试从 Redis 刷新
            if getattr(self, "_redis_enabled", False) and not self.account_list:
                try:
                    ids = await asyncio.to_thread(redis_list_account_ids)
                    if ids:
                        if SHUFFLE_ACCOUNTS_ON_START:
                            random.shuffle(ids)
                        self.account_list = list(ids)
                        # 重新初始化轮询指针，避免集中打到同一段
                        self._rr_index = random.randint(0, max(0, len(self.account_list) - 1))
                        logger.info(f"[MULTI] [ACCOUNT] {req_tag}从 Redis 刷新账号列表: {len(self.account_list)} 个")
                except Exception as e:
                    logger.warning(f"[MULTI] [ACCOUNT] {req_tag}从 Redis 刷新账号列表失败: {str(e)[:80]}")

            # 如果指定了账户ID
            if account_id:
                if account_id not in self.accounts:
                    account = await self._ensure_account_loaded(account_id, request_id)
                else:
                    account = self.accounts[account_id]
                if not account.should_retry():
                    raise HTTPException(503, f"Account {account_id} temporarily unavailable")
                return account

            if not self.account_list:
                raise HTTPException(503, "No available accounts")

            # Round-robin：避免每次都扫描 account_list（10w 账号会非常慢）
            if not hasattr(self, "_rr_index"):
                self._rr_index = 0

            attempts = 0
            total = len(self.account_list)
            while attempts < total:
                idx = self._rr_index % total
                candidate_id = self.account_list[idx]
                self._rr_index = (self._rr_index + 1) % total
                attempts += 1

                acc = self.accounts.get(candidate_id)
                if acc is None:
                    try:
                        acc = await self._ensure_account_loaded(candidate_id, request_id)
                    except HTTPException:
                        # 账号配置缺失/过期等，继续找下一个
                        continue

                if not acc.should_retry():
                    continue

                logger.info(f"[MULTI] [ACCOUNT] {req_tag}选择账户: {candidate_id}")
                return acc

            raise HTTPException(503, "No available accounts")

# ---------- 配置文件管理 ----------
ACCOUNTS_FILE = "accounts.json"

_redis_client = None

def _redis_accounts_keys() -> tuple[str, str]:
    """返回 (ids_list_key, cfg_hash_key)"""
    prefix = REDIS_KEY_PREFIX or "gb2api"
    return f"{prefix}:accounts:ids", f"{prefix}:accounts:cfg"

def get_redis_client():
    """获取 Redis 客户端（同步，针对 6 实例 + 1 秒 2w 并发优化）。"""
    global _redis_client
    if _redis_client is not None:
        return _redis_client
    if not REDIS_URL:
        raise ValueError("未配置 REDIS_URL")
    try:
        import redis  # type: ignore
    except Exception as e:
        raise RuntimeError("未安装 redis 依赖，请在 requirements.txt 安装 redis") from e

    # 极限并发优化：针对 6 实例 + 1 秒 2w 并发场景
    pool = redis.ConnectionPool.from_url(
        REDIS_URL,
        decode_responses=True,
        max_connections=10000,         # 大幅提升连接池（6 实例 * 每实例约 1500 连接）
        socket_keepalive=True,         # 保持连接活跃
        socket_connect_timeout=2,      # 连接超时缩短到 2 秒（更快失败）
        socket_timeout=3,              # 读写超时 3 秒（加快响应）
        retry_on_timeout=True,         # 超时自动重试
        retry_on_error=[redis.exceptions.ConnectionError, redis.exceptions.TimeoutError],
        retry=redis.retry.Retry(redis.backoff.ExponentialBackoff(base=0.01, cap=0.5), 3),  # 快速重试
        health_check_interval=30,      # 每 30 秒健康检查
        encoding='utf-8',
        encoding_errors='strict',
    )
    _redis_client = redis.Redis(connection_pool=pool)

    # 测试连接
    try:
        _redis_client.ping()
        logger.info(f"[REDIS] 连接池初始化成功 (max_connections={pool.max_connections}, 支持 6 实例)")
    except Exception as e:
        logger.error(f"[REDIS] 连接失败: {str(e)}")
        raise

    return _redis_client

def redis_list_account_ids() -> list:
    ids_key, _ = _redis_accounts_keys()
    r = get_redis_client()
    return r.lrange(ids_key, 0, -1) or []

def redis_get_account_ids_page(offset: int, limit: int) -> list[str]:
    ids_key, _ = _redis_accounts_keys()
    r = get_redis_client()
    start = max(0, offset)
    end = start + max(0, limit) - 1
    if limit <= 0:
        return []
    return r.lrange(ids_key, start, end) or []

def redis_get_accounts_by_ids(ids: list[str]) -> list[dict]:
    if not ids:
        return []
    _, cfg_key = _redis_accounts_keys()
    r = get_redis_client()
    raws = r.hmget(cfg_key, ids)
    accounts = []
    for raw in raws:
        if not raw:
            continue
        try:
            accounts.append(json.loads(raw))
        except Exception:
            continue
    return accounts

def redis_upsert_account_config(account_id: str, account_cfg: dict):
    ids_key, cfg_key = _redis_accounts_keys()
    r = get_redis_client()
    pipe = r.pipeline(transaction=True)
    pipe.hset(cfg_key, account_id, json.dumps(account_cfg, ensure_ascii=False))
    # 如果是新 id，则追加到 ids 列表
    pipe.lpos(ids_key, account_id)
    exists_pos = pipe.execute()[-1]
    if exists_pos is None:
        r.rpush(ids_key, account_id)

def redis_get_account_config(account_id: str) -> Optional[dict]:
    _, cfg_key = _redis_accounts_keys()
    r = get_redis_client()
    raw = r.hget(cfg_key, account_id)
    if not raw:
        return None
    try:
        return json.loads(raw)
    except Exception:
        return None

def redis_save_accounts(accounts_data: list):
    """写入 Redis（覆盖 ids 列表 + cfg hash）。"""
    ids_key, cfg_key = _redis_accounts_keys()
    r = get_redis_client()
    pipe = r.pipeline(transaction=True)
    pipe.delete(ids_key)
    pipe.delete(cfg_key)

    ids: list[str] = []
    for i, acc in enumerate(accounts_data, 1):
        if not isinstance(acc, dict):
            continue
        account_id = get_account_id(acc, i)
        ids.append(account_id)
        acc_to_store = dict(acc)
        acc_to_store["id"] = account_id
        pipe.hset(cfg_key, account_id, json.dumps(acc_to_store, ensure_ascii=False))

    if ids:
        pipe.rpush(ids_key, *ids)
    pipe.execute()
    logger.info(f"[CONFIG] 配置已写入 Redis: {len(ids)} 个账户")

def redis_delete_account(account_id: str):
    ids_key, cfg_key = _redis_accounts_keys()
    r = get_redis_client()
    pipe = r.pipeline(transaction=True)
    pipe.hdel(cfg_key, account_id)
    pipe.lrem(ids_key, 0, account_id)
    pipe.execute()
    logger.info(f"[CONFIG] Redis 删除账户: {account_id}")

# ---------- Redis 禁用状态管理 ----------

def redis_set_account_disabled(account_id: str, disabled_until: float, reason: str = ""):
    """设置账户禁用状态到 Redis（多实例共享）

    Args:
        account_id: 账户ID
        disabled_until: 禁用截止时间戳（Unix timestamp）
        reason: 禁用原因（如 "429限流" 或 "连续失败"）
    """
    if not REDIS_URL:
        return  # 未配置 Redis 时跳过

    try:
        prefix = REDIS_KEY_PREFIX or "gb2api"
        key = f"{prefix}:account:disabled:{account_id}"
        r = get_redis_client()

        # 存储禁用信息
        data = {
            "disabled_until": disabled_until,
            "reason": reason,
            "set_at": time.time()
        }

        # 计算过期时间（禁用时长 + 1小时缓冲）
        ttl = int(disabled_until - time.time()) + 3600
        if ttl > 0:
            r.setex(key, ttl, json.dumps(data))
            logger.info(f"[REDIS] 账户 {account_id} 禁用状态已写入 Redis，截止时间: {datetime.fromtimestamp(disabled_until, tz=timezone(timedelta(hours=8))).strftime('%Y-%m-%d %H:%M:%S')}")
    except Exception as e:
        logger.error(f"[REDIS] 写入账户禁用状态失败: {str(e)[:100]}")

def redis_get_account_disabled(account_id: str) -> Optional[dict]:
    """从 Redis 获取账户禁用状态

    Returns:
        dict: {"disabled_until": float, "reason": str, "set_at": float} 或 None
    """
    if not REDIS_URL:
        return None

    try:
        prefix = REDIS_KEY_PREFIX or "gb2api"
        key = f"{prefix}:account:disabled:{account_id}"
        r = get_redis_client()

        raw = r.get(key)
        if not raw:
            return None

        return json.loads(raw)
    except Exception as e:
        logger.error(f"[REDIS] 读取账户禁用状态失败: {str(e)[:100]}")
        return None

def redis_clear_account_disabled(account_id: str):
    """清除账户禁用状态（解锁账户）"""
    if not REDIS_URL:
        return

    try:
        prefix = REDIS_KEY_PREFIX or "gb2api"
        key = f"{prefix}:account:disabled:{account_id}"
        r = get_redis_client()
        r.delete(key)
        logger.info(f"[REDIS] 账户 {account_id} 禁用状态已清除")
    except Exception as e:
        logger.error(f"[REDIS] 清除账户禁用状态失败: {str(e)[:100]}")

def save_accounts_to_file(accounts_data: list):
    """保存账户配置到文件"""
    with open(ACCOUNTS_FILE, 'w', encoding='utf-8') as f:
        json.dump(accounts_data, f, ensure_ascii=False, indent=2)
    logger.info(f"[CONFIG] 配置已保存到 {ACCOUNTS_FILE}")

def load_accounts_from_source() -> list:
    """优先从文件加载，否则从环境变量加载；可选从 Redis 加载"""
    if ACCOUNTS_SOURCE == "redis":
        ids = redis_list_account_ids()
        if not ids:
            raise ValueError("Redis 中未找到账号配置（ids 为空）")
        _, cfg_key = _redis_accounts_keys()
        r = get_redis_client()
        # HMGET 一次性取回（注意：100k 会很大，建议管理端使用分页或外部工具维护）
        raws = r.hmget(cfg_key, ids)
        accounts_data = []
        for raw in raws:
            if not raw:
                continue
            try:
                accounts_data.append(json.loads(raw))
            except Exception:
                continue
        logger.info(f"[CONFIG] 从 Redis 加载配置: {len(accounts_data)} 个账户")
        return accounts_data

    # 优先从文件加载
    if os.path.exists(ACCOUNTS_FILE):
        try:
            with open(ACCOUNTS_FILE, 'r', encoding='utf-8') as f:
                accounts_data = json.load(f)
            logger.info(f"[CONFIG] 从文件加载配置: {ACCOUNTS_FILE}")
            return accounts_data
        except Exception as e:
            logger.warning(f"[CONFIG] 文件加载失败，尝试环境变量: {str(e)}")

    # 从环境变量加载
    accounts_json = os.getenv("ACCOUNTS_CONFIG")
    if not accounts_json:
        raise ValueError(
            "未找到配置文件或 ACCOUNTS_CONFIG 环境变量。\n"
            "请在环境变量中配置 JSON 格式的账户列表，格式示例：\n"
            '[{"id":"account_1","csesidx":"xxx","config_id":"yyy","secure_c_ses":"zzz","host_c_oses":null,"expires_at":"2025-12-23 10:59:21"}]'
        )

    try:
        accounts_data = json.loads(accounts_json)
        if not isinstance(accounts_data, list):
            raise ValueError("ACCOUNTS_CONFIG 必须是 JSON 数组格式")
        # 首次从环境变量加载后，保存到文件
        save_accounts_to_file(accounts_data)
        logger.info(f"[CONFIG] 从环境变量加载配置并保存到文件")
        return accounts_data
    except json.JSONDecodeError as e:
        logger.error(f"[CONFIG] ACCOUNTS_CONFIG JSON 解析失败: {str(e)}")
        raise ValueError(f"ACCOUNTS_CONFIG 格式错误: {str(e)}")

def get_account_id(acc: dict, index: int) -> str:
    """获取账户ID（有显式ID则使用，否则生成默认ID）"""
    return acc.get("id", f"account_{index}")

# ---------- 多账户配置加载 ----------
def load_multi_account_config() -> MultiAccountManager:
    """从文件或环境变量加载多账户配置"""
    manager = MultiAccountManager()

    if ACCOUNTS_SOURCE == "redis":
        ids = redis_list_account_ids()
        if not ids:
            logger.warning("[CONFIG] Redis 中未找到账号配置（ids 为空），将以空账号列表启动（可后续通过管理端添加）")
            manager._redis_enabled = True  # type: ignore[attr-defined]
            manager._redis_invalid_ids = set()  # type: ignore[attr-defined]
            manager.account_list = []
            return manager
        if SHUFFLE_ACCOUNTS_ON_START:
            random.shuffle(ids)
        # 先填充 ID 列表（不预加载配置）
        manager.account_list = list(ids)
        manager._redis_enabled = True  # type: ignore[attr-defined]
        manager._redis_invalid_ids = set()  # type: ignore[attr-defined]

        preload_n = max(0, min(REDIS_ACCOUNTS_PRELOAD, len(ids)))
        if preload_n:
            # 预加载一部分账号到内存，便于管理面板预览 & 加速首请求
            _, cfg_key = _redis_accounts_keys()
            r = get_redis_client()
            raws = r.hmget(cfg_key, ids[:preload_n])
            accounts_data = []
            for raw in raws:
                if not raw:
                    continue
                try:
                    accounts_data.append(json.loads(raw))
                except Exception:
                    continue
        else:
            accounts_data = []
    else:
        accounts_data = load_accounts_from_source()

    for i, acc in enumerate(accounts_data, 1):
        # 验证必需字段
        required_fields = ["secure_c_ses", "csesidx", "config_id"]
        missing_fields = [f for f in required_fields if f not in acc]
        if missing_fields:
            raise ValueError(f"账户 {i} 缺少必需字段: {', '.join(missing_fields)}")

        config = AccountConfig(
            account_id=get_account_id(acc, i),
            secure_c_ses=acc["secure_c_ses"],
            host_c_oses=acc.get("host_c_oses"),
            csesidx=acc["csesidx"],
            config_id=acc["config_id"],
            expires_at=acc.get("expires_at")
        )

        # 检查账户是否已过期
        if config.is_expired():
            logger.warning(f"[CONFIG] 账户 {config.account_id} 已过期，跳过加载")
            continue

        manager.add_account(config)

    if not manager.account_list:
        # file 模式必须有账号；redis 模式允许空启动（便于后续在线添加）
        if ACCOUNTS_SOURCE == "redis":
            logger.warning("[CONFIG] 当前账号列表为空（Redis 模式允许空启动）")
            return manager
        raise ValueError("没有有效的账户配置（可能全部已过期）")

    logger.info(
        f"[CONFIG] 成功加载账户列表: {len(manager.account_list)} 个"
        + (f"（已预加载 {len(manager.accounts)} 个）" if ACCOUNTS_SOURCE == "redis" else "")
    )
    return manager


# 初始化多账户管理器
multi_account_mgr = load_multi_account_config()

def reload_accounts():
    """重新加载账户配置（清空缓存并重新加载）"""
    global multi_account_mgr
    multi_account_mgr.global_session_cache.clear()
    multi_account_mgr = load_multi_account_config()
    logger.info(f"[CONFIG] 配置已重载，当前账户数: {len(multi_account_mgr.accounts)}")

def update_accounts_config(accounts_data: list):
    """更新账户配置（保存到文件并重新加载）"""
    if ACCOUNTS_SOURCE == "redis":
        redis_save_accounts(accounts_data)
    else:
        save_accounts_to_file(accounts_data)
    reload_accounts()

def delete_account(account_id: str):
    """删除单个账户（轻量级删除，不触发全局重载）"""
    if ACCOUNTS_SOURCE == "redis":
        # Redis 模式：直接删除并从内存中移除
        redis_delete_account(account_id)

        # 从内存中移除（轻量级操作）
        if account_id in multi_account_mgr.accounts:
            del multi_account_mgr.accounts[account_id]
        if account_id in multi_account_mgr.account_list:
            multi_account_mgr.account_list.remove(account_id)

        # 清理该账号的会话缓存
        keys_to_remove = [
            key for key, value in multi_account_mgr.global_session_cache.items()
            if value.get("account_id") == account_id
        ]
        for key in keys_to_remove:
            del multi_account_mgr.global_session_cache[key]

        logger.info(f"[CONFIG] 账户 {account_id} 已从内存中移除（Redis 模式）")
        return

    accounts_data = load_accounts_from_source()

    # 过滤掉要删除的账户
    filtered = [
        acc for i, acc in enumerate(accounts_data, 1)
        if get_account_id(acc, i) != account_id
    ]

    if len(filtered) == len(accounts_data):
        raise ValueError(f"账户 {account_id} 不存在")

    save_accounts_to_file(filtered)

    # File 模式：轻量级删除，只从内存中移除
    if account_id in multi_account_mgr.accounts:
        del multi_account_mgr.accounts[account_id]
    if account_id in multi_account_mgr.account_list:
        multi_account_mgr.account_list.remove(account_id)

    # 清理该账号的会话缓存
    keys_to_remove = [
        key for key, value in multi_account_mgr.global_session_cache.items()
        if value.get("account_id") == account_id
    ]
    for key in keys_to_remove:
        del multi_account_mgr.global_session_cache[key]

def auto_delete_invalid_account(account_id: str, reason: str):
    """自动删除无效账号（401/503 错误时调用）

    Args:
        account_id: 账户ID
        reason: 删除原因（如 "401未授权" 或 "503服务不可用"）
    """
    try:
        logger.warning(f"[ACCOUNT] [{account_id}] 检测到无效账号（{reason}），正在自动删除...")
        delete_account(account_id)
        logger.info(f"[ACCOUNT] [{account_id}] 无效账号已自动删除（原因: {reason}）")
    except Exception as e:
        logger.error(f"[ACCOUNT] [{account_id}] 自动删除失败: {str(e)[:100]}")

# 验证必需的环境变量
if not PATH_PREFIX:
    logger.error("[SYSTEM] 未配置 PATH_PREFIX 环境变量，请设置后重启")
    import sys
    sys.exit(1)

if not ADMIN_KEY:
    logger.error("[SYSTEM] 未配置 ADMIN_KEY 环境变量，请设置后重启")
    import sys
    sys.exit(1)

# 启动日志
logger.info(f"[SYSTEM] 路径前缀已配置: {PATH_PREFIX[:4]}****")
logger.info(f"[SYSTEM] 用户端点: /{PATH_PREFIX}/v1/chat/completions")
logger.info(f"[SYSTEM] 管理端点: /{PATH_PREFIX}/admin/")
logger.info("[SYSTEM] 公开端点: /public/log/html")
logger.info("[SYSTEM] 系统初始化完成")

# ---------- JWT 管理 ----------
class JWTManager:
    def __init__(self, config: AccountConfig) -> None:
        self.config = config
        self.jwt: str = ""
        self.expires: float = 0
        self._lock = asyncio.Lock()

    async def get(self, request_id: str = "") -> str:
        async with self._lock:
            if time.time() > self.expires:
                await self._refresh(request_id)
            return self.jwt

    async def _refresh(self, request_id: str = "") -> None:
        cookie = f"__Secure-C_SES={self.config.secure_c_ses}"
        if self.config.host_c_oses:
            cookie += f"; __Host-C_OSES={self.config.host_c_oses}"

        req_tag = f"[req_{request_id}] " if request_id else ""
        r = await http_client.get(
            "https://business.gemini.google/auth/getoxsrf",
            params={"csesidx": self.config.csesidx},
            headers={
                "cookie": cookie,
                "user-agent": USER_AGENT,
                "referer": "https://business.gemini.google/"
            },
        )
        if r.status_code != 200:
            logger.error(f"[AUTH] [{self.config.account_id}] {req_tag}JWT 刷新失败: {r.status_code}")

            # 401/503 错误时自动删除无效账号
            if r.status_code in [401, 503]:
                auto_delete_invalid_account(
                    self.config.account_id,
                    f"JWT刷新{r.status_code}错误"
                )

            raise HTTPException(r.status_code, "getoxsrf failed")

        txt = r.text[4:] if r.text.startswith(")]}'") else r.text
        data = json.loads(txt)

        key_bytes = base64.urlsafe_b64decode(data["xsrfToken"] + "==")
        self.jwt      = create_jwt(key_bytes, data["keyId"], self.config.csesidx)
        self.expires = time.time() + 270
        logger.info(f"[AUTH] [{self.config.account_id}] {req_tag}JWT 刷新成功")

# ---------- Session & File 管理 ----------
async def create_google_session(account_manager: AccountManager, request_id: str = "") -> str:
    jwt = await account_manager.get_jwt(request_id)
    headers = get_common_headers(jwt)
    body = {
        "configId": account_manager.config.config_id,
        "additionalParams": {"token": "-"},
        "createSessionRequest": {
            "session": {"name": "", "displayName": ""}
        }
    }

    req_tag = f"[req_{request_id}] " if request_id else ""
    r = await http_client.post(
        "https://biz-discoveryengine.googleapis.com/v1alpha/locations/global/widgetCreateSession",
        headers=headers,
        json=body,
    )
    if r.status_code != 200:
        logger.error(f"[SESSION] [{account_manager.config.account_id}] {req_tag}Session 创建失败: {r.status_code}")

        # 401/503 错误时自动删除账号
        if r.status_code in [401, 503]:
            auto_delete_invalid_account(
                account_manager.config.account_id,
                f"{r.status_code}错误"
            )

        raise HTTPException(r.status_code, "createSession failed")
    sess_name = r.json()["session"]["name"]
    logger.info(f"[SESSION] [{account_manager.config.account_id}] {req_tag}创建成功: {sess_name[-12:]}")
    return sess_name

async def upload_context_file(session_name: str, mime_type: str, base64_content: str, account_manager: AccountManager, request_id: str = "") -> str:
    """上传文件到指定 Session，返回 fileId"""
    jwt = await account_manager.get_jwt(request_id)
    headers = get_common_headers(jwt)

    # 生成随机文件名
    ext = mime_type.split('/')[-1] if '/' in mime_type else "bin"
    file_name = f"upload_{int(time.time())}_{uuid.uuid4().hex[:6]}.{ext}"

    body = {
        "configId": account_manager.config.config_id,
        "additionalParams": {"token": "-"},
        "addContextFileRequest": {
            "name": session_name,
            "fileName": file_name,
            "mimeType": mime_type,
            "fileContents": base64_content
        }
    }

    r = await http_client.post(
        "https://biz-discoveryengine.googleapis.com/v1alpha/locations/global/widgetAddContextFile",
        headers=headers,
        json=body,
    )

    req_tag = f"[req_{request_id}] " if request_id else ""
    if r.status_code != 200:
        logger.error(f"[FILE] [{account_manager.config.account_id}] {req_tag}文件上传失败: {r.status_code}")
        raise HTTPException(r.status_code, f"Upload failed: {r.text}")

    data = r.json()
    file_id = data.get("addContextFileResponse", {}).get("fileId")
    logger.info(f"[FILE] [{account_manager.config.account_id}] {req_tag}文件上传成功: {mime_type}")
    return file_id

# ---------- 消息处理逻辑 ----------
def get_conversation_key(messages: List[dict]) -> str:
    """使用第一条user消息生成对话指纹"""
    if not messages:
        return "empty"

    # 只使用第一条user消息生成指纹（对话起点不变）
    user_messages = [msg for msg in messages if msg.get("role") == "user"]
    if not user_messages:
        return "no_user_msg"

    # 只取第一条user消息
    first_user_msg = user_messages[0]
    content = first_user_msg.get("content", "")

    # 统一处理内容格式（字符串或数组）
    if isinstance(content, list):
        text = "".join([x.get("text", "") for x in content if x.get("type") == "text"])
    else:
        text = str(content)

    # 标准化：去除首尾空白，转小写（避免因空格/大小写导致指纹不同）
    text = text.strip().lower()

    # 生成指纹
    return hashlib.md5(text.encode()).hexdigest()

def parse_last_message(messages: List['Message']):
    """解析最后一条消息，分离文本和图片"""
    if not messages:
        return "", []
    
    last_msg = messages[-1]
    content = last_msg.content
    
    text_content = ""
    images = [] # List of {"mime": str, "data": str_base64}

    if isinstance(content, str):
        text_content = content
    elif isinstance(content, list):
        for part in content:
            if part.get("type") == "text":
                text_content += part.get("text", "")
            elif part.get("type") == "image_url":
                url = part.get("image_url", {}).get("url", "")
                # 解析 Data URI: data:image/png;base64,xxxxxx
                match = re.match(r"data:(image/[^;]+);base64,(.+)", url)
                if match:
                    images.append({"mime": match.group(1), "data": match.group(2)})
                else:
                    logger.warning(f"[FILE] 不支持的图片格式: {url[:30]}...")

    return text_content, images

def build_full_context_text(messages: List['Message']) -> str:
    """仅拼接历史文本，图片只处理当次请求的"""
    prompt = ""
    for msg in messages:
        role = "User" if msg.role in ["user", "system"] else "Assistant"
        content_str = ""
        if isinstance(msg.content, str):
            content_str = msg.content
        elif isinstance(msg.content, list):
            for part in msg.content:
                if part.get("type") == "text":
                    content_str += part.get("text", "")
                elif part.get("type") == "image_url":
                    content_str += "[图片]"
        
        prompt += f"{role}: {content_str}\n\n"
    return prompt

def estimate_token_count(text: str) -> int:
    """非常粗略的 token 估算（仅用于兼容 OpenAI usage 字段）。"""
    if not text:
        return 0
    cjk_chars = sum(1 for ch in text if "\u4e00" <= ch <= "\u9fff")
    other_chars = len(text) - cjk_chars
    # 经验值：英文/符号大约 4 字符 ~ 1 token；中文大约 1 字 ~ 1 token（非常不精确）
    return cjk_chars + int(math.ceil(other_chars / 4))

# ---------- OpenAI 兼容接口 ----------
app = FastAPI(title="Gemini-Business OpenAI Gateway")
app.state.admin_session_ttl_seconds = ADMIN_SESSION_TTL_SECONDS


def is_admin_logged_in(request: Request) -> bool:
    """校验管理员 Cookie session（用于页面跳转逻辑）。"""
    if not ADMIN_KEY:
        return False
    cookie_value = request.cookies.get(ADMIN_SESSION_COOKIE_NAME)
    return verify_admin_session_cookie(cookie_value, ADMIN_KEY, ADMIN_SESSION_TTL_SECONDS)

# ---------- 图片静态服务初始化 ----------
os.makedirs(IMAGE_DIR, exist_ok=True)
app.mount("/images", StaticFiles(directory=IMAGE_DIR), name="images")
if IMAGE_DIR == "/data/images":
    logger.info(f"[SYSTEM] 图片静态服务已启用: /images/ -> {IMAGE_DIR} (持久化存储)")
else:
    logger.info(f"[SYSTEM] 图片静态服务已启用: /images/ -> {IMAGE_DIR} (临时存储，重启会丢失)")

# ---------- 后台任务启动 ----------
@app.on_event("startup")
async def startup_event():
    """应用启动时初始化后台任务"""
    # 启动缓存清理任务
    asyncio.create_task(multi_account_mgr.start_background_cleanup())
    logger.info("[SYSTEM] 后台缓存清理任务已启动（间隔: 5分钟）")

# ---------- 导入模板模块 ----------
# 注意：必须在所有全局变量初始化之后导入，避免循环依赖
from core import templates

# ---------- 日志脱敏函数 ----------
def get_sanitized_logs(limit: int = 100) -> list:
    """获取脱敏后的日志列表，按请求ID分组并提取关键事件"""
    with log_lock:
        logs = list(log_buffer)

    # 按请求ID分组（支持两种格式：带[req_xxx]和不带的）
    request_logs = {}
    orphan_logs = []  # 没有request_id的日志（如选择账户）

    for log in logs:
        message = log["message"]
        req_match = re.search(r'\[req_([a-z0-9]+)\]', message)

        if req_match:
            request_id = req_match.group(1)
            if request_id not in request_logs:
                request_logs[request_id] = []
            request_logs[request_id].append(log)
        else:
            # 没有request_id的日志（如选择账户），暂存
            orphan_logs.append(log)

    # 将orphan_logs（如选择账户）关联到对应的请求
    # 策略：将orphan日志关联到时间上最接近的后续请求
    for orphan in orphan_logs:
        orphan_time = orphan["time"]
        # 找到时间上最接近且在orphan之后的请求
        closest_request_id = None
        min_time_diff = None

        for request_id, req_logs in request_logs.items():
            if req_logs:
                first_log_time = req_logs[0]["time"]
                # orphan应该在请求之前或同时
                if first_log_time >= orphan_time:
                    if min_time_diff is None or first_log_time < min_time_diff:
                        min_time_diff = first_log_time
                        closest_request_id = request_id

        # 如果找到最接近的请求，将orphan日志插入到该请求的日志列表开头
        if closest_request_id:
            request_logs[closest_request_id].insert(0, orphan)

    # 为每个请求提取关键事件
    sanitized = []
    for request_id, req_logs in request_logs.items():
        # 收集关键信息
        model = None
        message_count = None
        retry_events = []
        final_status = "in_progress"
        duration = None
        start_time = req_logs[0]["time"]

        # 遍历该请求的所有日志
        for log in req_logs:
            message = log["message"]

            # 提取模型名称和消息数量（开始对话）
            if '收到请求:' in message and not model:
                model_match = re.search(r'收到请求: ([^ |]+)', message)
                if model_match:
                    model = model_match.group(1)
                count_match = re.search(r'(\d+)条消息', message)
                if count_match:
                    message_count = int(count_match.group(1))

            # 提取重试事件（包括失败尝试、账户切换、选择账户）
            # 注意：不提取"正在重试"日志，因为它和"失败 (尝试"是配套的
            if any(keyword in message for keyword in ['切换账户', '选择账户', '失败 (尝试']):
                retry_events.append({
                    "time": log["time"],
                    "message": message
                })

            # 提取响应完成（最高优先级 - 最终成功则忽略中间错误）
            if '响应完成:' in message:
                time_match = re.search(r'响应完成: ([\d.]+)秒', message)
                if time_match:
                    duration = time_match.group(1) + 's'
                    final_status = "success"

            # 检测非流式响应完成
            if '非流式响应完成' in message:
                final_status = "success"

            # 检测失败状态（仅在非success状态下）
            if final_status != "success" and (log['level'] == 'ERROR' or '失败' in message):
                final_status = "error"

            # 检测超时（仅在非success状态下）
            if final_status != "success" and '超时' in message:
                final_status = "timeout"

        # 如果没有模型信息但有错误，仍然显示
        if not model and final_status == "in_progress":
            continue

        # 构建关键事件列表
        events = []

        # 1. 开始对话
        if model:
            events.append({
                "time": start_time,
                "type": "start",
                "content": f"{model} | {message_count}条消息" if message_count else model
            })
        else:
            # 没有模型信息但有错误的情况
            events.append({
                "time": start_time,
                "type": "start",
                "content": "请求处理中"
            })

        # 2. 重试事件
        failure_count = 0  # 失败重试计数
        account_select_count = 0  # 账户选择计数

        for i, retry in enumerate(retry_events):
            msg = retry["message"]

            # 识别不同类型的重试事件（按优先级匹配）
            if '失败 (尝试' in msg:
                # 创建会话失败
                failure_count += 1
                events.append({
                    "time": retry["time"],
                    "type": "retry",
                    "content": f"服务异常，正在重试（{failure_count}）"
                })
            elif '选择账户' in msg:
                # 账户选择/切换
                account_select_count += 1

                # 检查下一条日志是否是"切换账户"，如果是则跳过当前"选择账户"（避免重复）
                next_is_switch = (i + 1 < len(retry_events) and '切换账户' in retry_events[i + 1]["message"])

                if not next_is_switch:
                    if account_select_count == 1:
                        # 第一次选择：显示为"选择服务节点"
                        events.append({
                            "time": retry["time"],
                            "type": "select",
                            "content": "选择服务节点"
                        })
                    else:
                        # 第二次及以后：显示为"切换服务节点"
                        events.append({
                            "time": retry["time"],
                            "type": "switch",
                            "content": "切换服务节点"
                        })
            elif '切换账户' in msg:
                # 运行时切换账户（显示为"切换服务节点"）
                events.append({
                    "time": retry["time"],
                    "type": "switch",
                    "content": "切换服务节点"
                })

        # 3. 完成事件
        if final_status == "success":
            if duration:
                events.append({
                    "time": req_logs[-1]["time"],
                    "type": "complete",
                    "status": "success",
                    "content": f"响应完成 | 耗时{duration}"
                })
            else:
                events.append({
                    "time": req_logs[-1]["time"],
                    "type": "complete",
                    "status": "success",
                    "content": "响应完成"
                })
        elif final_status == "error":
            events.append({
                "time": req_logs[-1]["time"],
                "type": "complete",
                "status": "error",
                "content": "请求失败"
            })
        elif final_status == "timeout":
            events.append({
                "time": req_logs[-1]["time"],
                "type": "complete",
                "status": "timeout",
                "content": "请求超时"
            })

        sanitized.append({
            "request_id": request_id,
            "start_time": start_time,
            "status": final_status,
            "events": events
        })

    # 按时间排序并限制数量
    sanitized.sort(key=lambda x: x["start_time"], reverse=True)
    return sanitized[:limit]

class Message(BaseModel):
    role: str
    content: Union[str, List[Dict[str, Any]]]

class ChatRequest(BaseModel):
    model: str = "gemini-auto"
    messages: List[Message]
    stream: bool = False
    temperature: Optional[float] = 0.7
    top_p: Optional[float] = 1.0


def gemini_contents_to_openai_messages(contents: list) -> List[Message]:
    """
    将 Gemini generateContent 的 contents/parts 格式转换为 OpenAI 兼容 messages。

    支持：
    - part.text
    - part.inlineData {mimeType, data(base64)}
    """
    messages: List[Message] = []
    if not contents:
        return messages

    for item in contents:
        role = (item.get("role") or "user").lower()
        parts = item.get("parts") or []

        # Gemini: user/model；OpenAI: user/assistant
        mapped_role = "assistant" if role in {"model", "assistant"} else "user"

        text_buf = ""
        mm_parts: List[Dict[str, Any]] = []

        for part in parts:
            if not isinstance(part, dict):
                continue
            if "text" in part and part.get("text") is not None:
                text_buf += str(part.get("text"))
                continue

            inline_data = part.get("inlineData") or part.get("inline_data")
            if inline_data:
                mime = inline_data.get("mimeType") or inline_data.get("mime_type") or "application/octet-stream"
                data = inline_data.get("data") or ""
                if data:
                    mm_parts.append({
                        "type": "image_url",
                        "image_url": {"url": f"data:{mime};base64,{data}"}
                    })

        if mm_parts:
            if text_buf:
                mm_parts.insert(0, {"type": "text", "text": text_buf})
            messages.append(Message(role=mapped_role, content=mm_parts))
        else:
            messages.append(Message(role=mapped_role, content=text_buf))

    return messages

def create_chunk(id: str, created: int, model: str, delta: dict, finish_reason: Union[str, None]) -> str:
    chunk = {
        "id": id,
        "object": "chat.completion.chunk",
        "created": created,
        "model": model,
        "choices": [{
            "index": 0,
            "delta": delta,
            "logprobs": None,  # OpenAI 标准字段
            "finish_reason": finish_reason
        }],
        "system_fingerprint": None  # OpenAI 标准字段（可选）
    }
    return json.dumps(chunk)

# ---------- API Key 验证 ----------
def verify_api_key(authorization: str = None, x_goog_api_key: str = None):
    """验证 API Key（如果配置了 API_KEY），支持 Authorization 或 x-goog-api-key"""
    # 如果未配置 API_KEY，则跳过验证
    if API_KEY is None:
        return True

    token = None
    if authorization:
        # 支持两种格式：
        # 1. Bearer YOUR_API_KEY
        # 2. YOUR_API_KEY
        token = authorization[7:] if authorization.startswith("Bearer ") else authorization
    elif x_goog_api_key:
        token = x_goog_api_key

    if not token:
        raise HTTPException(status_code=401, detail="Missing API key (Authorization or x-goog-api-key)")

    if token != API_KEY:
        logger.warning(f"[AUTH] API Key 验证失败")
        raise HTTPException(
            status_code=401,
            detail="Invalid API Key"
        )

    return True

@app.get("/")
async def home(request: Request):
    """首页 - 默认显示管理面板（可通过环境变量隐藏）"""
    # 检查是否隐藏首页
    if HIDE_HOME_PAGE:
        raise HTTPException(404, "Not Found")

    # 未登录则跳转到登录页
    if not is_admin_logged_in(request):
        return RedirectResponse(url=f"/{PATH_PREFIX}/login?next=/{PATH_PREFIX}/admin", status_code=302)

    # 显示管理页面（带隐藏提示）
    html_content = templates.generate_admin_html(request, multi_account_mgr, show_hide_tip=True)
    return HTMLResponse(content=html_content)


@app.get("/{path_prefix}/login")
@require_path_prefix(PATH_PREFIX)
async def admin_login_page(path_prefix: str, request: Request, next: str = None):
    """管理员登录页"""
    if not ADMIN_KEY:
        html = templates.generate_login_html(path_prefix, next or "", "服务端未配置 ADMIN_KEY，无法登录。")
        return HTMLResponse(content=html, status_code=500)
    html = templates.generate_login_html(path_prefix, next or "")
    return HTMLResponse(content=html)


@app.post("/{path_prefix}/login")
@require_path_prefix(PATH_PREFIX)
async def admin_login_submit(path_prefix: str, request: Request, admin_key: str = Form(...), next: str = Form(None)):
    """提交 ADMIN_KEY，写入 Cookie session"""
    if not ADMIN_KEY:
        html = templates.generate_login_html(path_prefix, next or "", "服务端未配置 ADMIN_KEY，无法登录。")
        return HTMLResponse(content=html, status_code=500)

    if admin_key != ADMIN_KEY:
        html = templates.generate_login_html(path_prefix, next or "", "ADMIN_KEY 错误")
        return HTMLResponse(content=html, status_code=401)

    default_next = f"/{PATH_PREFIX}/admin"
    target = next if (next and next.startswith(f"/{PATH_PREFIX}/")) else default_next
    resp = RedirectResponse(url=target, status_code=302)
    resp.set_cookie(
        ADMIN_SESSION_COOKIE_NAME,
        create_admin_session_cookie(ADMIN_KEY),
        httponly=True,
        samesite="lax",
        secure=(request.url.scheme == "https"),
        path=f"/{PATH_PREFIX}",
        max_age=ADMIN_SESSION_TTL_SECONDS,
    )
    return resp


@app.post("/{path_prefix}/logout")
@require_path_prefix(PATH_PREFIX)
async def admin_logout(path_prefix: str):
    """清除管理员 Cookie session"""
    resp = RedirectResponse(url=f"/{PATH_PREFIX}/login", status_code=302)
    resp.delete_cookie(ADMIN_SESSION_COOKIE_NAME, path=f"/{PATH_PREFIX}")
    return resp

@app.get("/{path_prefix}/admin")
@require_path_and_admin(PATH_PREFIX, ADMIN_KEY)
async def admin_home(path_prefix: str, request: Request, key: str = None, authorization: str = Header(None)):
    """管理首页 - 显示API信息和错误提醒"""
    # 显示管理页面（不显示隐藏提示）
    html_content = templates.generate_admin_html(request, multi_account_mgr, show_hide_tip=False)
    return HTMLResponse(content=html_content)

@app.get("/{path_prefix}/v1/models")
@require_path_prefix(PATH_PREFIX)
async def list_models(path_prefix: str, authorization: str = Header(None), x_goog_api_key: Optional[str] = Header(None, alias="x-goog-api-key")):
    # 验证 API Key
    verify_api_key(authorization, x_goog_api_key)

    data = []
    now = int(time.time())
    for m in MODEL_MAPPING.keys():
        data.append({
            "id": m,
            "object": "model",
            "created": now,
            "owned_by": "google",
            "permission": []
        })
    return {"object": "list", "data": data}

@app.get("/{path_prefix}/v1/models/{model_id}")
@require_path_prefix(PATH_PREFIX)
async def get_model(path_prefix: str, model_id: str, authorization: str = Header(None), x_goog_api_key: Optional[str] = Header(None, alias="x-goog-api-key")):
    # 验证 API Key
    verify_api_key(authorization, x_goog_api_key)

    return {"id": model_id, "object": "model"}

@app.get("/{path_prefix}/admin/health")
@require_path_and_admin(PATH_PREFIX, ADMIN_KEY)
async def admin_health(path_prefix: str, request: Request, key: str = None, authorization: str = Header(None)):
    return {"status": "ok", "time": datetime.utcnow().isoformat()}

@app.get("/{path_prefix}/admin/accounts")
@require_path_and_admin(PATH_PREFIX, ADMIN_KEY)
async def admin_get_accounts(
    path_prefix: str,
    request: Request,
    offset: int = 0,
    limit: int = 200,
    search: str = None,
    key: str = None,
    authorization: str = Header(None),
):
    """获取账户状态信息（Redis 模式：分页/搜索；file 模式：全量）"""
    if ACCOUNTS_SOURCE != "redis":
        accounts_info = []
        for account_id, account_manager in multi_account_mgr.accounts.items():
            config = account_manager.config
            remaining_hours = config.get_remaining_hours()
            status, status_color, remaining_display = format_account_expiration(remaining_hours)
            accounts_info.append({
                "id": config.account_id,
                "status": status,
                "expires_at": config.expires_at or "未设置",
                "remaining_hours": remaining_hours,
                "remaining_display": remaining_display,
                "is_available": account_manager.is_available,
                "error_count": account_manager.error_count
            })
        return {"total": len(accounts_info), "accounts": accounts_info, "offset": 0, "limit": len(accounts_info)}

    limit = min(max(1, limit), 1000)
    offset = max(0, offset)

    if search:
        ids = await asyncio.to_thread(redis_list_account_ids)
        matched = [i for i in ids if search in i]
        page_ids = matched[offset:offset + limit]
        cfgs = await asyncio.to_thread(redis_get_accounts_by_ids, page_ids)
        total = len(matched)
    else:
        page_ids = await asyncio.to_thread(redis_get_account_ids_page, offset, limit)
        cfgs = await asyncio.to_thread(redis_get_accounts_by_ids, page_ids)
        total = len(await asyncio.to_thread(redis_list_account_ids))

    accounts_info = []
    for cfg in cfgs:
        acc_id = cfg.get("id")
        # 如果已被懒加载到内存，则可提供更准确的可用性/失败次数
        mgr = multi_account_mgr.accounts.get(acc_id) if acc_id else None

        config = AccountConfig(
            account_id=acc_id or "",
            secure_c_ses=cfg.get("secure_c_ses", ""),
            host_c_oses=cfg.get("host_c_oses"),
            csesidx=cfg.get("csesidx", ""),
            config_id=cfg.get("config_id", ""),
            expires_at=cfg.get("expires_at")
        )
        remaining_hours = config.get_remaining_hours()
        status, status_color, remaining_display = format_account_expiration(remaining_hours)

        accounts_info.append({
            "id": config.account_id,
            "status": status,
            "expires_at": config.expires_at or "未设置",
            "remaining_hours": remaining_hours,
            "remaining_display": remaining_display,
            "is_available": mgr.is_available if mgr else True,
            "error_count": mgr.error_count if mgr else 0
        })

    return {"total": total, "accounts": accounts_info, "offset": offset, "limit": limit, "search": search}

@app.get("/{path_prefix}/admin/accounts-config")
@require_path_and_admin(PATH_PREFIX, ADMIN_KEY)
async def admin_get_config(
    path_prefix: str,
    request: Request,
    offset: int = 0,
    limit: int = 200,
    search: str = None,
    key: str = None,
    authorization: str = Header(None),
):
    """获取账户配置（文件模式：全量；Redis 模式：分页/搜索）"""
    try:
        if ACCOUNTS_SOURCE != "redis":
            accounts_data = load_accounts_from_source()
            return {"accounts": accounts_data, "total": len(accounts_data), "offset": 0, "limit": len(accounts_data)}

        # Redis 模式：默认分页
        if limit <= 0:
            limit = 200
        limit = min(limit, 1000)
        offset = max(offset, 0)

        if search:
            # 简单按 account_id 子串匹配（会扫描 ids，适合管理端）
            ids = await asyncio.to_thread(redis_list_account_ids)
            matched = [i for i in ids if search in i]
            page_ids = matched[offset:offset + limit]
            accounts = await asyncio.to_thread(redis_get_accounts_by_ids, page_ids)
            return {"accounts": accounts, "total": len(matched), "offset": offset, "limit": limit, "search": search}

        page_ids = await asyncio.to_thread(redis_get_account_ids_page, offset, limit)
        accounts = await asyncio.to_thread(redis_get_accounts_by_ids, page_ids)
        total = len(await asyncio.to_thread(redis_list_account_ids))
        return {"accounts": accounts, "total": total, "offset": offset, "limit": limit}
    except Exception as e:
        logger.error(f"[CONFIG] 获取配置失败: {str(e)}")
        raise HTTPException(500, f"获取失败: {str(e)}")


@app.get("/{path_prefix}/admin/accounts-config/{account_id}")
@require_path_and_admin(PATH_PREFIX, ADMIN_KEY)
async def admin_get_config_by_id(
    path_prefix: str,
    account_id: str,
    request: Request,
    key: str = None,
    authorization: str = Header(None),
):
    """按 ID 获取单个账户配置（Redis 模式推荐）"""
    try:
        if ACCOUNTS_SOURCE == "redis":
            cfg = await asyncio.to_thread(redis_get_account_config, account_id)
            if not cfg:
                raise HTTPException(404, "Not Found")
            return {"account": cfg}
        # file 模式：从列表中查找
        accounts_data = load_accounts_from_source()
        for i, acc in enumerate(accounts_data, 1):
            if get_account_id(acc, i) == account_id:
                return {"account": acc}
        raise HTTPException(404, "Not Found")
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"[CONFIG] 获取单个配置失败: {str(e)}")
        raise HTTPException(500, f"获取失败: {str(e)}")

@app.put("/{path_prefix}/admin/accounts-config")
@require_path_and_admin(PATH_PREFIX, ADMIN_KEY)
async def admin_update_config(path_prefix: str, request: Request, accounts_data: list = Body(...), key: str = None, authorization: str = Header(None)):
    """更新整个账户配置"""
    try:
        update_accounts_config(accounts_data)
        return {"status": "success", "message": "配置已更新", "account_count": len(multi_account_mgr.accounts)}
    except Exception as e:
        logger.error(f"[CONFIG] 更新配置失败: {str(e)}")
        raise HTTPException(500, f"更新失败: {str(e)}")


@app.put("/{path_prefix}/admin/accounts-config/{account_id}")
@require_path_and_admin(PATH_PREFIX, ADMIN_KEY)
async def admin_upsert_config_by_id(
    path_prefix: str,
    account_id: str,
    request: Request,
    account: dict = Body(...),
    key: str = None,
    authorization: str = Header(None),
):
    """按 ID 更新/新增单个账户配置（Redis 模式推荐）"""
    try:
        required_fields = ["secure_c_ses", "csesidx", "config_id"]
        missing_fields = [f for f in required_fields if f not in account]
        if missing_fields:
            raise HTTPException(400, f"缺少必需字段: {', '.join(missing_fields)}")

        account_to_store = dict(account)
        account_to_store["id"] = account_id

        if ACCOUNTS_SOURCE == "redis":
            await asyncio.to_thread(redis_upsert_account_config, account_id, account_to_store)
            reload_accounts()
            return {"status": "success", "message": "配置已更新", "account_id": account_id}

        # file 模式：更新列表中的对应项（若不存在则追加）
        accounts_data = load_accounts_from_source()
        updated = False
        for i, acc in enumerate(accounts_data, 1):
            if get_account_id(acc, i) == account_id:
                accounts_data[i - 1] = account_to_store
                updated = True
                break
        if not updated:
            accounts_data.append(account_to_store)
        update_accounts_config(accounts_data)
        return {"status": "success", "message": "配置已更新", "account_id": account_id}
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"[CONFIG] 按ID更新配置失败: {str(e)}")
        raise HTTPException(500, f"更新失败: {str(e)}")

@app.delete("/{path_prefix}/admin/accounts/{account_id}")
@require_path_and_admin(PATH_PREFIX, ADMIN_KEY)
async def admin_delete_account(path_prefix: str, account_id: str, request: Request, key: str = None, authorization: str = Header(None)):
    """删除单个账户"""
    try:
        delete_account(account_id)
        return {"status": "success", "message": f"账户 {account_id} 已删除", "account_count": len(multi_account_mgr.accounts)}
    except Exception as e:
        logger.error(f"[CONFIG] 删除账户失败: {str(e)}")
        raise HTTPException(500, f"删除失败: {str(e)}")

@app.post("/{path_prefix}/admin/accounts")
@require_path_and_admin(PATH_PREFIX, ADMIN_KEY)
async def admin_add_account(
    path_prefix: str,
    request: Request,
    account: dict = Body(...),
    key: str = None,
    authorization: str = Header(None),
):
    """
    新增单个账号（推荐用于 Redis 大规模账号模式）

    Body(JSON):
    - id (可选): 账号ID，不传则自动生成
    - secure_c_ses (必需)
    - csesidx (必需)
    - config_id (必需)
    - host_c_oses (可选)
    - expires_at (可选)
    """
    try:
        if not isinstance(account, dict):
            raise HTTPException(400, "Body must be a JSON object")

        required_fields = ["secure_c_ses", "csesidx", "config_id"]
        missing_fields = [f for f in required_fields if not account.get(f)]
        if missing_fields:
            raise HTTPException(400, f"缺少必需字段: {', '.join(missing_fields)}")

        account_id = (account.get("id") or "").strip() or f"account_{uuid.uuid4().hex[:10]}"
        account_to_store = dict(account)
        account_to_store["id"] = account_id

        # 过期检查（过期账号不允许新增）
        cfg = AccountConfig(
            account_id=account_id,
            secure_c_ses=account_to_store["secure_c_ses"],
            host_c_oses=account_to_store.get("host_c_oses"),
            csesidx=account_to_store["csesidx"],
            config_id=account_to_store["config_id"],
            expires_at=account_to_store.get("expires_at"),
        )
        if cfg.is_expired():
            raise HTTPException(400, "账号已过期（expires_at），请修正后再添加")

        if ACCOUNTS_SOURCE == "redis":
            await asyncio.to_thread(redis_upsert_account_config, account_id, account_to_store)

            # 更新内存中的 account_list（不强制 reload 以避免 10w 列表重载）
            if account_id not in multi_account_mgr.account_list:
                multi_account_mgr.account_list.append(account_id)

            return {
                "status": "success",
                "account_id": account_id,
                "message": "账号已添加（Redis）",
                "account_count": len(multi_account_mgr.account_list),
            }

        # file 模式：追加到列表并重载
        accounts_data = load_accounts_from_source()
        accounts_data.append(account_to_store)
        update_accounts_config(accounts_data)
        return {
            "status": "success",
            "account_id": account_id,
            "message": "账号已添加（file）",
            "account_count": len(multi_account_mgr.account_list),
        }
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"[CONFIG] 新增账户失败: {str(e)}")
        raise HTTPException(500, f"新增失败: {str(e)}")

@app.get("/{path_prefix}/admin/log")
@require_path_and_admin(PATH_PREFIX, ADMIN_KEY)
async def admin_get_logs(
    path_prefix: str,
    request: Request,
    limit: int = 1500,
    key: str = None,
    authorization: str = Header(None),
    level: str = None,
    search: str = None,
    start_time: str = None,
    end_time: str = None
):
    """
    获取系统日志（包含统计信息）

    参数:
    - limit: 返回最近 N 条日志 (默认 1500, 最大 3000)
    - level: 过滤日志级别 (INFO, WARNING, ERROR, DEBUG)
    - search: 搜索关键词（在消息中搜索）
    - start_time: 开始时间 (格式: 2025-12-17 10:00:00)
    - end_time: 结束时间 (格式: 2025-12-17 11:00:00)
    """
    with log_lock:
        logs = list(log_buffer)

    # 计算统计信息（在过滤前）
    stats_by_level = {}
    error_logs = []
    chat_count = 0
    for log in logs:
        level_name = log.get("level", "INFO")
        stats_by_level[level_name] = stats_by_level.get(level_name, 0) + 1

        # 收集错误日志
        if level_name in ["ERROR", "CRITICAL"]:
            error_logs.append(log)

        # 统计对话次数（匹配包含"收到请求"的日志）
        if "收到请求" in log.get("message", ""):
            chat_count += 1

    # 按级别过滤
    if level:
        level = level.upper()
        logs = [log for log in logs if log["level"] == level]

    # 按关键词搜索
    if search:
        logs = [log for log in logs if search.lower() in log["message"].lower()]

    # 按时间范围过滤
    if start_time:
        logs = [log for log in logs if log["time"] >= start_time]
    if end_time:
        logs = [log for log in logs if log["time"] <= end_time]

    # 限制数量（返回最近的）
    limit = min(limit, 3000)
    filtered_logs = logs[-limit:]

    return {
        "total": len(filtered_logs),
        "limit": limit,
        "filters": {
            "level": level,
            "search": search,
            "start_time": start_time,
            "end_time": end_time
        },
        "logs": filtered_logs,
        "stats": {
            "memory": {
                "total": len(log_buffer),
                "by_level": stats_by_level,
                "capacity": log_buffer.maxlen
            },
            "errors": {
                "count": len(error_logs),
                "recent": error_logs[-10:]  # 最近10条错误
            },
            "chat_count": chat_count
        }
    }

@app.delete("/{path_prefix}/admin/log")
@require_path_and_admin(PATH_PREFIX, ADMIN_KEY)
async def admin_clear_logs(path_prefix: str, request: Request, confirm: str = None, key: str = None, authorization: str = Header(None)):
    """
    清空所有日志（内存缓冲 + 文件）

    参数:
    - confirm: 必须传入 "yes" 才能清空
    """
    if confirm != "yes":
        raise HTTPException(
            status_code=400,
            detail="需要 confirm=yes 参数确认清空操作"
        )

    # 清空内存缓冲
    with log_lock:
        cleared_count = len(log_buffer)
        log_buffer.clear()

    logger.info("[LOG] 日志已清空")

    return {
        "status": "success",
        "message": "已清空内存日志",
        "cleared_count": cleared_count
    }

@app.get("/{path_prefix}/admin/log/html")
@require_path_and_admin(PATH_PREFIX, ADMIN_KEY)
async def admin_logs_html_route(path_prefix: str, request: Request, key: str = None, authorization: str = Header(None)):
    """返回美化的 HTML 日志查看界面"""
    return await templates.admin_logs_html(path_prefix)

@app.post("/{path_prefix}/v1/chat/completions")
@require_path_prefix(PATH_PREFIX)
async def chat(
    path_prefix: str,
    req: ChatRequest,
    request: Request,
    authorization: Optional[str] = Header(None),
    x_goog_api_key: Optional[str] = Header(None, alias="x-goog-api-key"),
):
    # 1. API Key 验证
    verify_api_key(authorization, x_goog_api_key)

    # 1. 生成请求ID（最优先，用于所有日志追踪）
    request_id = str(uuid.uuid4())[:6]

    # 记录请求统计
    with stats_lock:
        global_stats["total_requests"] += 1
        global_stats["request_timestamps"].append(time.time())
        save_stats(global_stats)

    # 2. 模型校验
    if req.model not in MODEL_MAPPING:
        logger.error(f"[CHAT] [req_{request_id}] 不支持的模型: {req.model}")
        raise HTTPException(
            status_code=404,
            detail=f"Model '{req.model}' not found. Available models: {list(MODEL_MAPPING.keys())}"
        )

    # 2.1 账号校验：允许 Redis 空启动，但实际调用必须有可用账号
    if not getattr(multi_account_mgr, "account_list", None):
        if ACCOUNTS_SOURCE == "redis":
            try:
                ids = await asyncio.to_thread(redis_list_account_ids)
                if ids:
                    if SHUFFLE_ACCOUNTS_ON_START:
                        random.shuffle(ids)
                    multi_account_mgr.account_list = list(ids)
            except Exception:
                pass
        if not getattr(multi_account_mgr, "account_list", None):
            logger.error(f"[CHAT] [req_{request_id}] 当前未配置任何账号")
            raise HTTPException(status_code=503, detail="No accounts configured. Please add accounts first.")

    # 3. 会话策略：默认复用会话；若关闭复用则每次请求创建新会话
    conv_key = get_conversation_key([m.dict() for m in req.messages]) if ENABLE_SESSION_REUSE else f"no_reuse_{request_id}"
    cached_session = multi_account_mgr.global_session_cache.get(conv_key) if ENABLE_SESSION_REUSE else None

    account_manager = None
    google_session = None
    is_new_conversation = True

    if cached_session:
        # 使用已绑定的账户/Session
        account_id = cached_session["account_id"]
        account_manager = await multi_account_mgr.get_account(account_id, request_id)
        google_session = cached_session["session_id"]
        is_new_conversation = False
        logger.info(f"[CHAT] [{account_id}] [req_{request_id}] 继续会话: {google_session[-12:]}")
    else:
        # 新对话：轮询选择可用账户，失败时尝试其他账户
        # 注意：Redis 模式为懒加载，accounts(dict) 可能为空，必须用 account_list 计算尝试次数
        max_account_tries = min(MAX_NEW_SESSION_TRIES, len(multi_account_mgr.account_list))
        last_error = None

        for attempt in range(max_account_tries):
            try:
                account_manager = await multi_account_mgr.get_account(None, request_id)
                google_session = await create_google_session(account_manager, request_id)
                # 仅在启用会话复用时写入缓存
                if ENABLE_SESSION_REUSE:
                    await multi_account_mgr.set_session_cache(
                        conv_key,
                        account_manager.config.account_id,
                        google_session
                    )
                is_new_conversation = True
                logger.info(
                    f"[CHAT] [{account_manager.config.account_id}] [req_{request_id}] 新会话创建"
                    + ("并绑定账户" if ENABLE_SESSION_REUSE else "（未启用会话复用）")
                )
                break
            except Exception as e:
                last_error = e
                error_type = type(e).__name__
                # 安全获取账户ID
                account_id = account_manager.config.account_id if 'account_manager' in locals() and account_manager else 'unknown'

                # 检查是否是 401/503 错误，自动删除无效账号
                if isinstance(e, HTTPException) and e.status_code in [401, 503]:
                    auto_delete_invalid_account(account_id, f"{e.status_code}错误")

                logger.error(f"[CHAT] [req_{request_id}] 账户 {account_id} 创建会话失败 (尝试 {attempt + 1}/{max_account_tries}) - {error_type}: {str(e)}")
                if attempt == max_account_tries - 1:
                    logger.error(f"[CHAT] [req_{request_id}] 所有账户均不可用")
                    raise HTTPException(503, f"All accounts unavailable: {str(last_error)[:100]}")
                # 继续尝试下一个账户

        if not account_manager or not google_session:
            logger.error(f"[CHAT] [req_{request_id}] 未能创建可用会话（无可用账户）")
            raise HTTPException(503, "No available accounts")

    # 提取用户消息内容用于日志
    if req.messages:
        last_content = req.messages[-1].content
        if isinstance(last_content, str):
            # 显示完整消息，但限制在500字符以内
            if len(last_content) > 500:
                preview = last_content[:500] + "...(已截断)"
            else:
                preview = last_content
        else:
            preview = f"[多模态: {len(last_content)}部分]"
    else:
        preview = "[空消息]"

    # 记录请求基本信息
    logger.info(f"[CHAT] [{account_manager.config.account_id}] [req_{request_id}] 收到请求: {req.model} | {len(req.messages)}条消息 | stream={req.stream}")

    # 单独记录用户消息内容（方便查看）
    logger.info(f"[CHAT] [{account_manager.config.account_id}] [req_{request_id}] 用户消息: {preview}")

    # 3. 解析请求内容
    last_text, current_images = parse_last_message(req.messages)

    # 4. 准备文本内容
    if is_new_conversation:
        # 新对话只发送最后一条
        text_to_send = last_text
        is_retry_mode = True
    else:
        # 继续对话只发送当前消息
        text_to_send = last_text
        is_retry_mode = False
        # 线程安全地更新时间戳（仅在启用会话复用时）
        if ENABLE_SESSION_REUSE:
            await multi_account_mgr.update_session_time(conv_key)

    chat_id = f"chatcmpl-{uuid.uuid4()}"
    created_time = int(time.time())

    # 封装生成器 (含图片上传和重试逻辑)
    async def response_wrapper():
        nonlocal account_manager, google_session  # 允许修改外层的 account_manager / session

        retry_count = 0
        max_retries = MAX_REQUEST_RETRIES  # 使用配置的最大重试次数

        current_text = text_to_send
        current_retry_mode = is_retry_mode

        # 图片 ID 列表 (每次 Session 变化都需要重新上传，因为 fileId 绑定在 Session 上)
        current_file_ids = []

        # 记录已失败的账户，避免重复使用
        failed_accounts = set()

        # 重试逻辑：最多尝试 max_retries+1 次（初次+重试）
        while retry_count <= max_retries:
            try:
                if ENABLE_SESSION_REUSE:
                    # 安全：使用.get()防止缓存被清理导致KeyError
                    cached = multi_account_mgr.global_session_cache.get(conv_key)
                    if not cached:
                        logger.warning(f"[CHAT] [{account_manager.config.account_id}] [req_{request_id}] 缓存已清理，重建Session")
                        new_sess = await create_google_session(account_manager, request_id)
                        await multi_account_mgr.set_session_cache(
                            conv_key,
                            account_manager.config.account_id,
                            new_sess
                        )
                        google_session = new_sess
                        current_session = new_sess
                        current_retry_mode = True
                        current_file_ids = []
                    else:
                        current_session = cached["session_id"]
                        google_session = current_session
                else:
                    # 不复用会话：本次请求始终使用当前请求创建的 session
                    current_session = google_session
                    if not current_session:
                        current_session = await create_google_session(account_manager, request_id)
                        google_session = current_session
                        current_retry_mode = True
                        current_file_ids = []

                # A. 如果有图片且还没上传到当前 Session，先上传
                # 注意：每次重试如果是新 Session，都需要重新上传图片
                if current_images and not current_file_ids:
                    for img in current_images:
                        fid = await upload_context_file(current_session, img["mime"], img["data"], account_manager, request_id)
                        current_file_ids.append(fid)

                # B. 准备文本 (重试模式下发全文)
                if current_retry_mode:
                    current_text = build_full_context_text(req.messages)

                # C. 发起对话
                async for chunk in stream_chat_generator(
                    current_session,
                    current_text,
                    current_file_ids,
                    req.model,
                    chat_id,
                    created_time,
                    account_manager,
                    req.stream,
                    request_id,
                    request
                ):
                    yield chunk

                # 请求成功，重置账户失败计数
                account_manager.is_available = True
                account_manager.error_count = 0
                break

            except (httpx.ConnectError, httpx.ReadTimeout, ssl.SSLError, HTTPException) as e:
                # 记录当前失败的账户
                failed_accounts.add(account_manager.config.account_id)

                # 特殊处理 401/503/429 错误：自动删除无效账号
                if isinstance(e, HTTPException) and e.status_code in [401, 429, 503]:
                    auto_delete_invalid_account(
                        account_manager.config.account_id,
                        f"{e.status_code}错误"
                    )
                    # 继续重试其他账号
                    retry_count += 1
                    if retry_count <= max_retries:
                        logger.warning(f"[CHAT] [req_{request_id}] 账号已删除，正在重试 ({retry_count}/{max_retries})")
                        continue
                    else:
                        if req.stream: yield f"data: {json.dumps({'error': {'message': f'Account Invalid ({e.status_code})'}})}\n\n"
                        return

                # 普通错误：增加账户失败计数（触发熔断机制）
                account_manager.last_error_time = time.time()
                account_manager.error_count += 1
                if account_manager.error_count >= ACCOUNT_FAILURE_THRESHOLD:
                    account_manager.is_available = False

                    # 写入 Redis（多实例共享禁用状态）
                    # 普通失败禁用时长使用 ACCOUNT_COOLDOWN_SECONDS
                    disabled_until = time.time() + ACCOUNT_COOLDOWN_SECONDS
                    redis_set_account_disabled(
                        account_manager.config.account_id,
                        disabled_until,
                        f"连续失败{account_manager.error_count}次"
                    )

                    logger.error(f"[ACCOUNT] [{account_manager.config.account_id}] [req_{request_id}] 请求连续失败{account_manager.error_count}次，账户已标记为不可用")

                retry_count += 1

                # 详细记录错误信息
                error_type = type(e).__name__
                error_detail = str(e)

                # 特殊处理HTTPException，提取状态码和详情
                if isinstance(e, HTTPException):
                    logger.error(f"[CHAT] [{account_manager.config.account_id}] [req_{request_id}] HTTP错误 {e.status_code}: {e.detail}")
                else:
                    logger.error(f"[CHAT] [{account_manager.config.account_id}] [req_{request_id}] {error_type}: {error_detail}")

                # 检查是否还能继续重试
                if retry_count <= max_retries:
                    logger.warning(f"[CHAT] [{account_manager.config.account_id}] [req_{request_id}] 正在重试 ({retry_count}/{max_retries})")
                    # 尝试切换到其他账户（客户端会传递完整上下文）
                    try:
                        # 获取新账户，跳过已失败的账户
                        max_account_tries = MAX_ACCOUNT_SWITCH_TRIES  # 使用配置的账户切换尝试次数
                        new_account = None

                        for _ in range(max_account_tries):
                            candidate = await multi_account_mgr.get_account(None, request_id)
                            if candidate.config.account_id not in failed_accounts:
                                new_account = candidate
                                break

                        if not new_account:
                            logger.error(f"[CHAT] [req_{request_id}] 所有账户均已失败，无可用账户")
                            if req.stream: yield f"data: {json.dumps({'error': {'message': 'All Accounts Failed'}})}\n\n"
                            return

                        logger.info(f"[CHAT] [req_{request_id}] 切换账户: {account_manager.config.account_id} -> {new_account.config.account_id}")

                        # 创建新 Session
                        new_sess = await create_google_session(new_account, request_id)

                        # 更新缓存绑定到新账户（仅在启用复用时）
                        if ENABLE_SESSION_REUSE:
                            await multi_account_mgr.set_session_cache(
                                conv_key,
                                new_account.config.account_id,
                                new_sess
                            )

                        # 更新账户管理器 / session
                        account_manager = new_account
                        google_session = new_sess

                        # 设置重试模式（发送完整上下文）
                        current_retry_mode = True
                        current_file_ids = []  # 清空 ID，强制重新上传到新 Session

                    except Exception as create_err:
                        error_type = type(create_err).__name__
                        logger.error(f"[CHAT] [req_{request_id}] 账户切换失败 ({error_type}): {str(create_err)}")
                        if req.stream: yield f"data: {json.dumps({'error': {'message': 'Account Failover Failed'}})}\n\n"
                        return
                else:
                    # 已达到最大重试次数
                    logger.error(f"[CHAT] [req_{request_id}] 已达到最大重试次数 ({max_retries})，请求失败")
                    if req.stream: yield f"data: {json.dumps({'error': {'message': f'Max retries ({max_retries}) exceeded: {e}'}})}\n\n"
                    return

    if req.stream:
        return StreamingResponse(response_wrapper(), media_type="text/event-stream")
    
    full_content = ""
    full_reasoning = ""
    async for chunk_str in response_wrapper():
        if chunk_str.startswith("data: [DONE]"): break
        if chunk_str.startswith("data: "):
            try:
                data = json.loads(chunk_str[6:])
                delta = data["choices"][0]["delta"]
                if "content" in delta:
                    full_content += delta["content"]
                if "reasoning_content" in delta:
                    full_reasoning += delta["reasoning_content"]
            except json.JSONDecodeError as e:
                logger.error(f"[CHAT] [{account_manager.config.account_id}] [req_{request_id}] JSON解析失败: {str(e)}")
            except (KeyError, IndexError) as e:
                logger.error(f"[CHAT] [{account_manager.config.account_id}] [req_{request_id}] 响应格式错误 ({type(e).__name__}): {str(e)}")

    # 构建响应消息
    message = {"role": "assistant"}
    if full_reasoning:
        message["reasoning_content"] = full_reasoning
    message["content"] = full_content

    # 非流式请求完成日志
    logger.info(f"[CHAT] [{account_manager.config.account_id}] [req_{request_id}] 非流式响应完成")

    # 记录响应内容（限制500字符）
    response_preview = full_content[:500] + "...(已截断)" if len(full_content) > 500 else full_content
    logger.info(f"[CHAT] [{account_manager.config.account_id}] [req_{request_id}] AI响应: {response_preview}")

    if ENABLE_USAGE_ESTIMATE:
        prompt_text = build_full_context_text(req.messages)
        prompt_tokens = estimate_token_count(prompt_text)
        completion_tokens = estimate_token_count(full_reasoning + full_content)
        usage = {
            "prompt_tokens": prompt_tokens,
            "completion_tokens": completion_tokens,
            "total_tokens": prompt_tokens + completion_tokens
        }
    else:
        usage = {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}

    return {
        "id": chat_id,
        "object": "chat.completion",
        "created": created_time,
        "model": req.model,
        "choices": [{"index": 0, "message": message, "finish_reason": "stop"}],
        "usage": usage
    }


# ---------- Gemini v1beta 兼容接口 ----------
def _map_openai_finish_reason_to_gemini(reason: Optional[str]) -> str:
    if not reason:
        return "STOP"
    r = reason.lower()
    if r in {"stop", "completed"}:
        return "STOP"
    if r in {"length"}:
        return "MAX_TOKENS"
    if r in {"content_filter"}:
        return "SAFETY"
    return "STOP"


def _openai_to_gemini_response(openai_resp: dict, prompt_text: str) -> dict:
    choice = (openai_resp.get("choices") or [{}])[0]
    msg = choice.get("message") or {}
    content_text = msg.get("content") or ""
    reasoning_text = msg.get("reasoning_content") or ""
    finish_reason = _map_openai_finish_reason_to_gemini(choice.get("finish_reason"))

    # 如果 OpenAI usage 是估算/为空，这里做 best-effort 兼容 Gemini usageMetadata
    prompt_tokens = estimate_token_count(prompt_text)
    thoughts_tokens = estimate_token_count(reasoning_text)
    candidates_tokens = estimate_token_count(content_text)
    total_tokens = prompt_tokens + thoughts_tokens + candidates_tokens

    return {
        "candidates": [{
            "content": {
                "role": "model",
                "parts": [{"text": content_text}]
            },
            "finishReason": finish_reason,
            "safetyRatings": []
        }],
        "usageMetadata": {
            "promptTokenCount": prompt_tokens,
            "candidatesTokenCount": candidates_tokens,
            "totalTokenCount": total_tokens,
            "thoughtsTokenCount": thoughts_tokens
        }
    }


@app.post("/v1beta/models/{model}:generateContent")
async def gemini_generate_content_root(
    model: str,
    request: Request,
    body: Dict[str, Any] = Body(...),
    authorization: Optional[str] = Header(None),
    x_goog_api_key: Optional[str] = Header(None, alias="x-goog-api-key"),
):
    """
    Gemini API 兼容：POST /v1beta/models/{model}:generateContent
    说明：内部仍走 OpenAI 兼容的对话逻辑，返回转换后的 candidates/usageMetadata。
    """
    verify_api_key(authorization, x_goog_api_key)
    contents = body.get("contents") or []
    messages = gemini_contents_to_openai_messages(contents)
    openai_req = ChatRequest(model=model, messages=messages, stream=False)
    openai_resp = await chat(path_prefix=PATH_PREFIX, req=openai_req, request=request, authorization=authorization, x_goog_api_key=x_goog_api_key)
    prompt_text = build_full_context_text(openai_req.messages)
    return _openai_to_gemini_response(openai_resp, prompt_text)


@app.post("/{path_prefix}/v1beta/models/{model}:generateContent")
@require_path_prefix(PATH_PREFIX)
async def gemini_generate_content_prefixed(
    path_prefix: str,
    model: str,
    request: Request,
    body: Dict[str, Any] = Body(...),
    authorization: Optional[str] = Header(None),
    x_goog_api_key: Optional[str] = Header(None, alias="x-goog-api-key"),
):
    """同上，带 PATH_PREFIX 的版本。"""
    verify_api_key(authorization, x_goog_api_key)
    contents = body.get("contents") or []
    messages = gemini_contents_to_openai_messages(contents)
    openai_req = ChatRequest(model=model, messages=messages, stream=False)
    openai_resp = await chat(path_prefix=PATH_PREFIX, req=openai_req, request=request, authorization=authorization, x_goog_api_key=x_goog_api_key)
    prompt_text = build_full_context_text(openai_req.messages)
    return _openai_to_gemini_response(openai_resp, prompt_text)


@app.post("/v1beta/models/{model}:streamGenerateContent")
async def gemini_stream_generate_content_root(
    model: str,
    request: Request,
    body: Dict[str, Any] = Body(...),
    authorization: Optional[str] = Header(None),
    x_goog_api_key: Optional[str] = Header(None, alias="x-goog-api-key"),
):
    """Gemini API 兼容：流式输出（SSE）"""
    verify_api_key(authorization, x_goog_api_key)
    contents = body.get("contents") or []
    messages = gemini_contents_to_openai_messages(contents)
    prompt_text = build_full_context_text(messages)

    openai_req = ChatRequest(model=model, messages=messages, stream=True)
    openai_stream = await chat(path_prefix=PATH_PREFIX, req=openai_req, request=request, authorization=authorization, x_goog_api_key=x_goog_api_key)

    if not isinstance(openai_stream, StreamingResponse):
        openai_resp = openai_stream
        return _openai_to_gemini_response(openai_resp, prompt_text)

    accept = (request.headers.get("accept") or "").lower()
    alt = (request.query_params.get("alt") or "").lower()
    stream_mode = "ndjson" if (alt == "ndjson" or "application/x-ndjson" in accept) else "sse"

    async def stream_wrapper():
        full_content = ""
        full_reasoning = ""

        async for chunk in openai_stream.body_iterator:
            chunk_str = chunk.decode("utf-8") if isinstance(chunk, (bytes, bytearray)) else str(chunk)

            if chunk_str.startswith("data: [DONE]"):
                final_payload = {
                    "candidates": [{
                        "content": {"role": "model", "parts": [{"text": ""}]},
                        "finishReason": "STOP",
                        "safetyRatings": []
                    }],
                    "usageMetadata": {
                        "promptTokenCount": estimate_token_count(prompt_text),
                        "candidatesTokenCount": estimate_token_count(full_content),
                        "totalTokenCount": estimate_token_count(prompt_text) + estimate_token_count(full_reasoning) + estimate_token_count(full_content),
                        "thoughtsTokenCount": estimate_token_count(full_reasoning),
                    }
                }
                if stream_mode == "ndjson":
                    yield f"{json.dumps(final_payload, ensure_ascii=False)}\n"
                else:
                    yield f"data: {json.dumps(final_payload, ensure_ascii=False)}\n\n"
                break

            if not chunk_str.startswith("data: "):
                continue

            try:
                data = json.loads(chunk_str[6:])
                delta = data["choices"][0]["delta"]
            except Exception:
                continue

            if "content" in delta:
                text = delta.get("content") or ""
                full_content += text
                payload = {
                    "candidates": [{
                        "content": {"role": "model", "parts": [{"text": text}]},
                        "safetyRatings": []
                    }]
                }
                if stream_mode == "ndjson":
                    yield f"{json.dumps(payload, ensure_ascii=False)}\n"
                else:
                    yield f"data: {json.dumps(payload, ensure_ascii=False)}\n\n"
            elif "reasoning_content" in delta:
                full_reasoning += (delta.get("reasoning_content") or "")

    media_type = "application/x-ndjson" if stream_mode == "ndjson" else "text/event-stream"
    return StreamingResponse(stream_wrapper(), media_type=media_type)


@app.post("/{path_prefix}/v1beta/models/{model}:streamGenerateContent")
@require_path_prefix(PATH_PREFIX)
async def gemini_stream_generate_content_prefixed(
    path_prefix: str,
    model: str,
    request: Request,
    body: Dict[str, Any] = Body(...),
    authorization: Optional[str] = Header(None),
    x_goog_api_key: Optional[str] = Header(None, alias="x-goog-api-key"),
):
    """Gemini API 兼容：流式输出（SSE），带 PATH_PREFIX 版本"""
    return await gemini_stream_generate_content_root(model=model, request=request, body=body, authorization=authorization, x_goog_api_key=x_goog_api_key)

# ---------- 图片生成处理函数 ----------
def parse_images_from_response(data_list: list) -> tuple[list, str]:
    """从API响应中解析图片文件引用
    返回: (file_ids_list, session_name)
    file_ids_list: [{"fileId": str, "mimeType": str}, ...]
    """
    file_ids = []
    session_name = ""

    for data in data_list:
        sar = data.get("streamAssistResponse")
        if not sar:
            continue

        # 获取session信息
        session_info = sar.get("sessionInfo", {})
        if session_info.get("session"):
            session_name = session_info["session"]

        answer = sar.get("answer") or {}
        replies = answer.get("replies") or []

        for reply in replies:
            gc = reply.get("groundedContent", {})
            content = gc.get("content", {})

            # 检查file字段（图片生成的关键）
            file_info = content.get("file")
            if file_info:
                logger.info(f"[IMAGE] [DEBUG] 发现file字段: {file_info}")
                if file_info.get("fileId"):
                    file_ids.append({
                        "fileId": file_info["fileId"],
                        "mimeType": file_info.get("mimeType", "image/png")
                    })

    return file_ids, session_name


async def get_session_file_metadata(account_mgr: AccountManager, session_name: str, request_id: str = "") -> dict:
    """获取session中的文件元数据，包括正确的session路径"""
    jwt = await account_mgr.get_jwt(request_id)
    headers = get_common_headers(jwt)
    body = {
        "configId": account_mgr.config.config_id,
        "additionalParams": {"token": "-"},
        "listSessionFileMetadataRequest": {
            "name": session_name,
            "filter": "file_origin_type = AI_GENERATED"
        }
    }

    resp = await http_client.post(
        "https://biz-discoveryengine.googleapis.com/v1alpha/locations/global/widgetListSessionFileMetadata",
        headers=headers,
        json=body
    )

    if resp.status_code == 401:
        # JWT过期，刷新后重试
        jwt = await account_mgr.get_jwt(request_id)
        headers = get_common_headers(jwt)
        resp = await http_client.post(
            "https://biz-discoveryengine.googleapis.com/v1alpha/locations/global/widgetListSessionFileMetadata",
            headers=headers,
            json=body
        )

    if resp.status_code != 200:
        logger.warning(f"[IMAGE] [{account_mgr.config.account_id}] [req_{request_id}] 获取文件元数据失败: {resp.status_code}")
        return {}

    data = resp.json()
    result = {}
    file_metadata_list = data.get("listSessionFileMetadataResponse", {}).get("fileMetadata", [])
    for fm in file_metadata_list:
        fid = fm.get("fileId")
        if fid:
            result[fid] = fm

    return result


def build_image_download_url(session_name: str, file_id: str) -> str:
    """构造图片下载URL"""
    return f"https://biz-discoveryengine.googleapis.com/v1alpha/{session_name}:downloadFile?fileId={file_id}&alt=media"


async def download_image_with_jwt(account_mgr: AccountManager, session_name: str, file_id: str, request_id: str = "") -> bytes:
    """使用JWT认证下载图片"""
    url = build_image_download_url(session_name, file_id)
    logger.info(f"[IMAGE] [DEBUG] 下载URL: {url}")
    logger.info(f"[IMAGE] [DEBUG] Session完整路径: {session_name}")
    jwt = await account_mgr.get_jwt(request_id)
    headers = get_common_headers(jwt)

    # 复用全局http_client
    resp = await http_client.get(url, headers=headers, follow_redirects=True)

    if resp.status_code == 401:
        # JWT过期，刷新后重试
        jwt = await account_mgr.get_jwt(request_id)
        headers = get_common_headers(jwt)
        resp = await http_client.get(url, headers=headers, follow_redirects=True)

    resp.raise_for_status()
    return resp.content


def save_image_to_hf(image_data: bytes, chat_id: str, file_id: str, mime_type: str, base_url: str) -> str:
    """保存图片到持久化存储,返回完整的公开URL"""
    ext_map = {"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp"}
    ext = ext_map.get(mime_type, ".png")

    filename = f"{chat_id}_{file_id}{ext}"
    save_path = os.path.join(IMAGE_DIR, filename)

    # 目录已在启动时创建(Line 635),无需重复创建
    with open(save_path, "wb") as f:
        f.write(image_data)

    return f"{base_url}/images/{filename}"

async def stream_chat_generator(session: str, text_content: str, file_ids: List[str], model_name: str, chat_id: str, created_time: int, account_manager: AccountManager, is_stream: bool = True, request_id: str = "", request: Request = None):
    start_time = time.time()

    # 记录发送给API的内容
    text_preview = text_content[:500] + "...(已截断)" if len(text_content) > 500 else text_content
    logger.info(f"[API] [{account_manager.config.account_id}] [req_{request_id}] 发送内容: {text_preview}")
    if file_ids:
        logger.info(f"[API] [{account_manager.config.account_id}] [req_{request_id}] 附带文件: {len(file_ids)}个")

    jwt = await account_manager.get_jwt(request_id)
    headers = get_common_headers(jwt)

    body = {
        "configId": account_manager.config.config_id,
        "additionalParams": {"token": "-"},
        "streamAssistRequest": {
            "session": session,
            "query": {"parts": [{"text": text_content}]},
            "filter": "",
            "fileIds": file_ids, # 注入文件 ID
            "answerGenerationMode": "NORMAL",
            "toolsSpec": {
                "webGroundingSpec": {},
                "toolRegistry": "default_tool_registry",
                "imageGenerationSpec": {},
                "videoGenerationSpec": {}
            },
            "languageCode": "zh-CN",
            "userMetadata": {"timeZone": "Asia/Shanghai"},
            "assistSkippingMode": "REQUEST_ASSIST"
        }
    }

    target_model_id = MODEL_MAPPING.get(model_name)
    if target_model_id:
        body["streamAssistRequest"]["assistGenerationConfig"] = {
            "modelId": target_model_id
        }

    if is_stream:
        chunk = create_chunk(chat_id, created_time, model_name, {"role": "assistant"}, None)
        yield f"data: {chunk}\n\n"

    # 使用流式请求
    async with http_client.stream(
        "POST",
        "https://biz-discoveryengine.googleapis.com/v1alpha/locations/global/widgetStreamAssist",
        headers=headers,
        json=body,
    ) as r:
        if r.status_code != 200:
            error_text = await r.aread()
            raise HTTPException(status_code=r.status_code, detail=f"Upstream Error {error_text.decode()}")

        # 使用异步解析器处理 JSON 数组流
        json_objects = []  # 收集所有响应对象用于图片解析
        has_content = False  # 标记是否收到有效内容

        try:
            async for json_obj in parse_json_array_stream_async(r.aiter_lines()):
                json_objects.append(json_obj)  # 收集响应

                # 检测上游错误响应
                if "error" in json_obj:
                    error_info = json_obj.get("error", {})
                    error_msg = error_info.get("message", "Unknown error")
                    error_code = error_info.get("code", "UNKNOWN")
                    logger.warning(f"[API] [{account_manager.config.account_id}] [req_{request_id}] 上游返回错误: {error_code} - {error_msg}，将切换账号重试")
                    raise HTTPException(status_code=502, detail=f"Upstream error: {error_code} - {error_msg}")

                # 提取文本内容
                for reply in json_obj.get("streamAssistResponse", {}).get("answer", {}).get("replies", []):
                    content_obj = reply.get("groundedContent", {}).get("content", {})
                    text = content_obj.get("text", "")

                    if not text:
                        continue

                    has_content = True  # 标记收到有效内容

                    # 区分思考过程和正常内容
                    if content_obj.get("thought"):
                        # 思考过程使用 reasoning_content 字段（类似 OpenAI o1）
                        chunk = create_chunk(chat_id, created_time, model_name, {"reasoning_content": text}, None)
                        yield f"data: {chunk}\n\n"
                    else:
                        # 正常内容使用 content 字段
                        chunk = create_chunk(chat_id, created_time, model_name, {"content": text}, None)
                        yield f"data: {chunk}\n\n"

            # 检测空响应：如果没有收到任何有效内容，抛出异常触发重试
            if not has_content and not json_objects:
                logger.warning(f"[API] [{account_manager.config.account_id}] [req_{request_id}] 上游返回空响应（无任何数据），将切换账号重试")
                raise HTTPException(status_code=502, detail="Empty response from upstream - no data received")

            if not has_content and json_objects:
                logger.warning(f"[API] [{account_manager.config.account_id}] [req_{request_id}] 上游返回空内容（收到 {len(json_objects)} 个对象但无文本），将切换账号重试")
                # 记录响应结构用于调试
                logger.debug(f"[API] [{account_manager.config.account_id}] [req_{request_id}] 响应结构: {json_objects[0] if json_objects else 'N/A'}")
                raise HTTPException(status_code=502, detail="No content in upstream response - empty text")

            # 处理图片生成
            if json_objects:
                logger.info(f"[IMAGE] [{account_manager.config.account_id}] [req_{request_id}] 开始解析图片，共{len(json_objects)}个响应对象")
                file_ids, session_name = parse_images_from_response(json_objects)
                logger.info(f"[IMAGE] [{account_manager.config.account_id}] [req_{request_id}] 解析结果: {len(file_ids)}张图片")
                logger.info(f"[IMAGE] [DEBUG] 响应中的session路径: {session_name}")

                if file_ids and session_name:
                    logger.info(f"[IMAGE] [{account_manager.config.account_id}] [req_{request_id}] 检测到{len(file_ids)}张生成图片")

                    try:
                        # 获取base_url
                        base_url = get_base_url(request) if request else ""
                        logger.info(f"[IMAGE] [DEBUG] 使用base_url: {base_url}")

                        # 获取文件元数据，找到正确的session路径
                        file_metadata = await get_session_file_metadata(account_manager, session_name, request_id)
                        logger.info(f"[IMAGE] [DEBUG] 获取到{len(file_metadata)}个文件元数据")

                        for idx, file_info in enumerate(file_ids, 1):
                            try:
                                fid = file_info["fileId"]
                                mime = file_info["mimeType"]

                                # 从元数据中获取正确的session路径
                                meta = file_metadata.get(fid, {})
                                correct_session = meta.get("session") or session_name
                                logger.info(f"[IMAGE] [DEBUG] 文件{fid}使用session: {correct_session}")

                                image_data = await download_image_with_jwt(account_manager, correct_session, fid, request_id)
                                image_url = save_image_to_hf(image_data, chat_id, fid, mime, base_url)
                                logger.info(f"[IMAGE] [{account_manager.config.account_id}] [req_{request_id}] 图片已保存: {image_url}")

                                # 返回Markdown格式图片
                                markdown = f"\n\n![生成的图片]({image_url})\n\n"
                                chunk = create_chunk(chat_id, created_time, model_name, {"content": markdown}, None)
                                yield f"data: {chunk}\n\n"
                            except Exception as e:
                                logger.error(f"[IMAGE] [{account_manager.config.account_id}] [req_{request_id}] 单张图片处理失败: {str(e)}")

                    except Exception as e:
                        logger.error(f"[IMAGE] [{account_manager.config.account_id}] [req_{request_id}] 图片处理失败: {str(e)}")

        except ValueError as e:
            logger.error(f"[API] [{account_manager.config.account_id}] [req_{request_id}] JSON解析失败: {str(e)}")
        except Exception as e:
            error_type = type(e).__name__
            logger.error(f"[API] [{account_manager.config.account_id}] [req_{request_id}] 流处理错误 ({error_type}): {str(e)}")
            raise

        total_time = time.time() - start_time
        logger.info(f"[API] [{account_manager.config.account_id}] [req_{request_id}] 响应完成: {total_time:.2f}秒")
    
    if is_stream:
        final_chunk = create_chunk(chat_id, created_time, model_name, {}, "stop")
        yield f"data: {final_chunk}\n\n"
        yield "data: [DONE]\n\n"

# ---------- 公开端点（无需认证） ----------
@app.get("/public/stats")
async def get_public_stats():
    """获取公开统计信息"""
    with stats_lock:
        # 清理1小时前的请求时间戳
        current_time = time.time()
        global_stats["request_timestamps"] = [
            ts for ts in global_stats["request_timestamps"]
            if current_time - ts < 3600
        ]

        # 计算每分钟请求数
        recent_minute = [
            ts for ts in global_stats["request_timestamps"]
            if current_time - ts < 60
        ]
        requests_per_minute = len(recent_minute)

        # 计算负载状态
        if requests_per_minute < 10:
            load_status = "low"
            load_color = "#10b981"  # 绿色
        elif requests_per_minute < 30:
            load_status = "medium"
            load_color = "#f59e0b"  # 黄色
        else:
            load_status = "high"
            load_color = "#ef4444"  # 红色

        return {
            "total_visitors": global_stats["total_visitors"],
            "total_requests": global_stats["total_requests"],
            "requests_per_minute": requests_per_minute,
            "load_status": load_status,
            "load_color": load_color
        }

@app.get("/public/log")
async def get_public_logs(request: Request, limit: int = 100):
    """获取脱敏后的日志（JSON格式）"""
    # 基于IP的访问统计（24小时内去重）
    # 优先从 X-Forwarded-For 获取真实IP（处理代理情况）
    client_ip = request.headers.get("x-forwarded-for")
    if client_ip:
        # X-Forwarded-For 可能包含多个IP，取第一个
        client_ip = client_ip.split(",")[0].strip()
    else:
        # 没有代理时使用直连IP
        client_ip = request.client.host if request.client else "unknown"

    current_time = time.time()

    with stats_lock:
        # 清理24小时前的IP记录
        if "visitor_ips" not in global_stats:
            global_stats["visitor_ips"] = {}

        expired_ips = [
            ip for ip, timestamp in global_stats["visitor_ips"].items()
            if current_time - timestamp > 86400  # 24小时
        ]
        for ip in expired_ips:
            del global_stats["visitor_ips"][ip]

        # 记录新访问（24小时内同一IP只计数一次）
        if client_ip not in global_stats["visitor_ips"]:
            global_stats["visitor_ips"][client_ip] = current_time
            global_stats["total_visitors"] = len(global_stats["visitor_ips"])
            save_stats(global_stats)

    sanitized_logs = get_sanitized_logs(limit=min(limit, 1000))
    return {
        "total": len(sanitized_logs),
        "logs": sanitized_logs
    }

@app.get("/public/log/html")
async def get_public_logs_html():
    """公开的脱敏日志查看器"""
    return await templates.get_public_logs_html()

# ---------- 全局 404 处理（必须在最后） ----------

@app.exception_handler(404)
async def not_found_handler(request: Request, exc: HTTPException):
    """全局 404 处理器"""
    return JSONResponse(
        status_code=404,
        content={"detail": "Not Found"}
    )

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=7860)
