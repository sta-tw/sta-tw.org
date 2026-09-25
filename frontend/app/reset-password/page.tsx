import type { Metadata } from "next";
import AuthLayout from "../components/auth-layout";
import ResetPasswordForm from "./reset-password-form";

export const metadata: Metadata = {
    title: "設定密碼 | S.T.A 特殊選才資源網",
    description: "設定 S.T.A 特殊選才資源網帳號的密碼。"
};

export default function ResetPasswordPage() {
    return (
        <AuthLayout titleId="reset-password-title">
            <ResetPasswordForm />
        </AuthLayout>
    );
}
