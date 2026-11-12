import {hashPassword, comparePassword} from "../../../src/app/auth/utils";

describe("comparePassword", () => {
    it("matches the hash produced by the hash password correctly", async () => {
        const password = "StrongPass"
        const hashedPassword = await hashPassword(password);

        expect(await comparePassword(password, hashedPassword)).toBe(true);
        expect(await comparePassword("wrongpassword", hashedPassword)).toBe(false);
    })
})