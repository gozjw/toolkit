import CryptoWorker from './crypto.worker.js?worker&inline';

const SALT_LENGTH = 16;
const IV_LENGTH = 12;

const pending = new Map();
let msgId = 0;
let worker = null;

function getWorker() {
  if (!worker) {
    worker = new CryptoWorker();
    worker.onmessage = (ev) => {
      const { id, ok, key, error } = ev.data;
      const handler = pending.get(id);
      if (!handler) return;
      pending.delete(id);
      if (ok) handler.resolve(key);
      else handler.reject(new Error(error));
    };
    worker.onerror = (err) => {
      pending.forEach(({ reject }) => reject(new Error(err.message || 'Worker 执行失败')));
      pending.clear();
      worker = null;
    };
  }
  return worker;
}

function deriveKeyFromPassword(password, salt) {
  return new Promise((resolve, reject) => {
    const id = ++msgId;
    pending.set(id, { resolve, reject });
    getWorker().postMessage({ id, type: 'derive', password, salt });
  });
}

function uint8ToBase64(u8) {
  return btoa(String.fromCharCode(...u8));
}

function base64ToUint8(b64) {
  return Uint8Array.from(atob(b64), c => c.charCodeAt(0));
}

export async function encryptData(plaintext, password) {
  const encoder = new TextEncoder();
  const plain = encoder.encode(plaintext);

  const salt = crypto.getRandomValues(new Uint8Array(SALT_LENGTH));
  const iv = crypto.getRandomValues(new Uint8Array(IV_LENGTH));

  const keyRaw = await deriveKeyFromPassword(password, salt);
  const cryptoKey = await crypto.subtle.importKey(
    'raw',
    keyRaw,
    { name: 'AES-GCM' },
    false,
    ['encrypt', 'decrypt']
  );

  const encryptedBuf = await crypto.subtle.encrypt(
    { name: 'AES-GCM', iv },
    cryptoKey,
    plain
  );
  const encrypted = new Uint8Array(encryptedBuf);

  const combined = new Uint8Array(salt.length + iv.length + encrypted.length);
  combined.set(salt, 0);
  combined.set(iv, salt.length);
  combined.set(encrypted, salt.length + iv.length);

  return uint8ToBase64(combined);
}

export async function decryptData(base64Ciphertext, password) {
  const combined = base64ToUint8(base64Ciphertext);
  const minLen = SALT_LENGTH + IV_LENGTH;
  if (combined.length < minLen) throw new Error('密文不完整');

  const salt = combined.slice(0, SALT_LENGTH);
  const iv = combined.slice(SALT_LENGTH, minLen);
  const cipherAndTag = combined.slice(minLen);

  const keyRaw = await deriveKeyFromPassword(password, salt);
  const cryptoKey = await crypto.subtle.importKey(
    'raw',
    keyRaw,
    { name: 'AES-GCM' },
    false,
    ['encrypt', 'decrypt']
  );

  try {
    const decBuf = await crypto.subtle.decrypt(
      { name: 'AES-GCM', iv },
      cryptoKey,
      cipherAndTag
    );
    return new TextDecoder().decode(decBuf);
  } catch {
    throw new Error('解密失败：密码错误或密文被篡改');
  }
}