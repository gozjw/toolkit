import { argon2id } from 'hash-wasm';

const ARGON2_PARAMS = {
    iterations: 3,
    memorySize: 16 * 1024,
    parallelism: 1,
};
const KEY_LENGTH = 32;

self.onmessage = async (ev) => {
    const { id, type, password, salt } = ev.data;
    if (type === 'derive') {
        try {
            const keyUint8 = await argon2id({
                password,
                salt,
                iterations: ARGON2_PARAMS.iterations,
                memorySize: ARGON2_PARAMS.memorySize,
                parallelism: ARGON2_PARAMS.parallelism,
                hashLength: KEY_LENGTH,
                outputType: 'binary',
            });
            self.postMessage({ id, ok: true, key: keyUint8 });
        } catch (err) {
            self.postMessage({ id, ok: false, error: err.message });
        }
    }
};
