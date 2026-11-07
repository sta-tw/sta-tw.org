import { Suspense } from "react";
import AccountPage from "../account-page";
import IdentityVerification from "./identity-verification";
import LinkedAccounts from "./linked-accounts";

export default function Page() {
    return (
        <AccountPage title="帳號驗證狀態" description="管理第三方帳號連動與身分驗證。">
            {/* LinkedAccounts reads ?oauth= from useSearchParams. */}
            <Suspense fallback={null}>
                <LinkedAccounts />
            </Suspense>
            <IdentityVerification />
        </AccountPage>
    );
}
