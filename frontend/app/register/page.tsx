import type { Metadata } from "next";
import AuthLayout from "../components/auth-layout";
import RegisterForm from "./register-form";

export const metadata: Metadata = {
    title: "註冊帳號 | S.T.A 特殊選才資源網",
    description: "建立 S.T.A 特殊選才資源網帳號。"
};

export default function RegisterPage() {
    return (
        <AuthLayout titleId="register-title">
            <RegisterForm />
        </AuthLayout>
    );
}
