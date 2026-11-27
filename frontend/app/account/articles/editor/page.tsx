import type { Metadata } from "next";
import { Suspense } from "react";
import { LoadingState } from "../../../components/status";
import ExperienceEditor from "./experience-editor";

export const metadata: Metadata = {
    title: "撰寫文章 | S.T.A 特殊選才資源網"
};

export default function Page() {
    return (
        // The editor reads ?id= from useSearchParams.
        <Suspense fallback={<LoadingState />}>
            <ExperienceEditor />
        </Suspense>
    );
}
