import type { Metadata } from "next";
import { Suspense } from "react";
import AuthLayout from "../components/auth-layout";
import LoginForm, { LoginFormFromParams } from "./login-form";

export const metadata: Metadata = {
    title: "登入帳號 | S.T.A 特殊選才資源網",
    description: "登入 S.T.A 特殊選才資源網。"
};

export default function LoginPage() {
    return (
        <AuthLayout titleId="login-title">
            {/* The fallback is the same form without ?next=/?oauth_error=, so
                the prerendered HTML is already usable before hydration. */}
            <Suspense fallback={<LoginForm />}>
                <LoginFormFromParams />
            </Suspense>
        </AuthLayout>
    );
}
