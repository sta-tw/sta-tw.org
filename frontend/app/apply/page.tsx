import type { Metadata } from "next";
import AuthLayout from "../components/auth-layout";
import ApplyForm from "./apply-form";

export const metadata: Metadata = {
    title: "帳號申請 | S.T.A 特殊選才資源網",
    description: "沒有學校信箱的自學生或特殊情形，申請 S.T.A 特殊選才資源網帳號。"
};

export default function ApplyPage() {
    return (
        <AuthLayout titleId="apply-title">
            <ApplyForm />
        </AuthLayout>
    );
}
