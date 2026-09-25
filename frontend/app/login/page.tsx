import type { Metadata } from "next";
import AuthLayout from "../components/auth-layout";
import LoginForm from "./login-form";

export const metadata: Metadata = {
    title: "登入帳號 | S.T.A 特殊選才資源網",
    description: "登入 S.T.A 特殊選才資源網。"
};

export default function LoginPage() {
    return (
        <AuthLayout titleId="login-title">
            <LoginForm />
        </AuthLayout>
    );
}
