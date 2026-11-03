import { describe, expect, test } from "bun:test";

import { sha256Bytes, sha256Hex } from "@/lib/sha256";

const hex = (bytes: Uint8Array) => Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");

describe("sha256 fallback for plain-HTTP intranet pages", () => {
    test("matches the FIPS 180-4 test vectors", () => {
        const encode = (text: string) => new TextEncoder().encode(text);
        expect(hex(sha256Bytes(encode("")))).toBe("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855");
        expect(hex(sha256Bytes(encode("abc")))).toBe("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
        expect(hex(sha256Bytes(encode("abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq")))).toBe("248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1");
    });

    test("equals Web Crypto across block boundaries and binary data", async () => {
        for (const length of [1, 55, 56, 63, 64, 65, 119, 120, 1000, 200_003]) {
            const bytes = new Uint8Array(length);
            for (let index = 0; index < length; index++) bytes[index] = (index * 131 + length) & 0xff;
            const expected = hex(new Uint8Array(await crypto.subtle.digest("SHA-256", bytes)));
            expect(hex(sha256Bytes(bytes))).toBe(expected);
            expect(await sha256Hex(new Blob([bytes]))).toBe(expected);
        }
        expect(await sha256Hex("generation-retry\0group\0task")).toBe(hex(sha256Bytes(new TextEncoder().encode("generation-retry\0group\0task"))));
    });
});
