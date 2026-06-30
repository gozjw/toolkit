import { argon2id } from 'hash-wasm';

// 1. 配置符合前端环境平衡的 Argon2id 参数
const ARGON2_PARAMS = {
  iterations: 3,         // 迭代 3 轮
  memorySize: 16 * 1024, // 16MB 内存（折中移动端性能，防止前端网页卡死）
  parallelism: 1,        // 浏览器端推荐单线程
};

const KEY_LENGTH = 32;   // AES-256 密钥长度 (32 字节)
const SALT_LENGTH = 16;  // 盐值 (16 字节)
const IV_LENGTH = 12;    // GCM 模式初始化向量推荐 12 字节

/**
 * 将十六进制字符串转为 Uint8Array
 */
function hexToUint8Array(hexString) {
  const result = new Uint8Array(hexString.length / 2);
  for (let i = 0; i < hexString.length; i += 2) {
    result[i / 2] = parseInt(hexString.substring(i, i + 2), 16);
  }
  return result;
}

/**
 * 将超大 Uint8Array 安全、无崩溃地转换为 Base64 
 * 通过浏览器底层 Blob 机制
 */
function safeUint8ArrayToBase64(uint8Array) {
  const blob = new Blob([uint8Array]);
  return new Promise((resolve) => {
    const reader = new FileReader();
    reader.onloadend = () => {
      const dataUrl = reader.result;
      resolve(dataUrl.substring(dataUrl.indexOf(',') + 1));
    };
    reader.readAsDataURL(blob);
  });
}

/**
 * 将 Base64 安全高效地转回 Uint8Array
 * 使用高性能的流式 fetch 替代可能卡顿的循环
 */
async function safeBase64ToUint8Array(base64String) {
  const res = await fetch(`data:application/octet-stream;base64,${base64String}`);
  const buffer = await res.arrayBuffer();
  return new Uint8Array(buffer);
}

/**
 * 2. 核心函数：利用 hash-wasm 派生 256 位密钥
 */
async function deriveKeyFromPassword(password, salt) {
  // 生成十六进制格式的哈希，并指定输出长度为 32 字节
  const hashHex = await argon2id({
    password: password,
    salt: salt,
    iterations: ARGON2_PARAMS.iterations,
    memorySize: ARGON2_PARAMS.memorySize,
    parallelism: ARGON2_PARAMS.parallelism,
    hashLength: KEY_LENGTH,
  });
  
  const keyUint8 = hexToUint8Array(hashHex);

  // 将派生出的字节数组导入为 Web Crypto API 可用的 CryptoKey 对象
  return await window.crypto.subtle.importKey(
    'raw',
    keyUint8,
    { name: 'AES-GCM' },
    false,
    ['encrypt', 'decrypt']
  );
}

/**
 * 3. 统一加密函数
 * @returns {Promise<string>} 返回 Base64 编码的打包密文
 */
export async function encryptData(plaintext, password) {
  const encoder = new TextEncoder();
  const plaintextUint8 = encoder.encode(plaintext);

  // 随机生成 Salt 和 IV
  const salt = window.crypto.getRandomValues(new Uint8Array(SALT_LENGTH));
  const iv = window.crypto.getRandomValues(new Uint8Array(IV_LENGTH));

  // 派生 AES 密钥
  const cryptoKey = await deriveKeyFromPassword(password, salt);

  // 使用浏览器硬件加速执行 AES-256-GCM 加密
  const encryptedBuffer = await window.crypto.subtle.encrypt(
    { name: 'AES-GCM', iv: iv, tagLength: 128 }, // 128位认证标签 (16字节)
    cryptoKey,
    plaintextUint8
  );

  const encryptedUint8 = new Uint8Array(encryptedBuffer);

  // 拼接封装数据：[16B Salt] + [12B IV] + [密文+Tag]
  const combined = new Uint8Array(salt.length + iv.length + encryptedUint8.length);
  combined.set(salt, 0);
  combined.set(iv, salt.length);
  combined.set(encryptedUint8, salt.length + iv.length);

  // 改用安全的异步 Blob 转换，不再引发 Maximum call stack size exceeded
  return await safeUint8ArrayToBase64(combined);
}

/**
 * 4. 统一解密函数
 * @param {string} base64Ciphertext 打包的 Base64 密文
 * @returns {Promise<string>} 还原的明文
 */
export async function decryptData(base64Ciphertext, password) {
  // 将 Base64 转回 Uint8Array
  const combined = await safeBase64ToUint8Array(base64Ciphertext);

  const minLength = SALT_LENGTH + IV_LENGTH;
  if (combined.length < minLength) {
    throw new Error('密文数据不完整');
  }

  // 严格根据字节长度切分提取
  const salt = combined.slice(0, SALT_LENGTH);
  const iv = combined.slice(SALT_LENGTH, SALT_LENGTH + IV_LENGTH);
  const encryptedData = combined.slice(minLength);

  // 用相同的密码和提取的盐重新恢复密钥
  const cryptoKey = await deriveKeyFromPassword(password, salt);

  try {
    // 浏览器硬件加速解密，并自动校验 GCM 认证标签（抗篡改）
    const decryptedBuffer = await window.crypto.subtle.decrypt(
      { name: 'AES-GCM', iv: iv, tagLength: 128 },
      cryptoKey,
      encryptedData
    );

    const decoder = new TextDecoder();
    return decoder.decode(decryptedBuffer);
  } catch (err) {
    throw new Error('解密失败：密码错误或密文在传输中被篡改！');
  }
}
