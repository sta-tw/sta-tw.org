"use client";

import Image from "next/image";
import { type ChangeEvent, useEffect, useRef, useState } from "react";
import Button from "../components/button";
import {
    articleOverviewFallbackPath,
    getAdminArticleOverviewImageMeta,
    getArticleOverviewImageUrl,
    resetAdminArticleOverviewImage,
    uploadAdminArticleOverviewImage,
    type SiteAssetMetadata
} from "../lib/api/site-assets";
import { publicPath } from "../lib/public-path";
import { useAdmin } from "./admin-context";
import { AdminPanel, Badge, describeError, ErrorText, formatDate, LoadingState } from "./admin-ui";

const fallbackArticleOverviewImage = publicPath(articleOverviewFallbackPath);
const maxImageBytes = 5 * 1024 * 1024;

export default function ArticleOverviewImageEditor() {
    const { mfaCode } = useAdmin();
    const fileInputRef = useRef<HTMLInputElement>(null);
    const [imageUrl, setImageUrl] = useState(fallbackArticleOverviewImage);
    const [metadata, setMetadata] = useState<SiteAssetMetadata | null>(null);
    const [hasCustomImage, setHasCustomImage] = useState(false);
    const [selectedFile, setSelectedFile] = useState<File | null>(null);
    const [localPreview, setLocalPreview] = useState<string | null>(null);
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [notice, setNotice] = useState<string | null>(null);

    useEffect(() => {
        let ignore = false;
        getAdminArticleOverviewImageMeta(mfaCode || undefined)
            .then(({ data }) => {
                if (ignore) return;
                setMetadata(data);
                setHasCustomImage(data.exists);
                setImageUrl(
                    data.exists
                        ? getArticleOverviewImageUrl(data.updated_at)
                        : fallbackArticleOverviewImage
                );
            })
            .catch((cause) => {
                if (!ignore) setError(describeError(cause));
            })
            .finally(() => {
                if (!ignore) setLoading(false);
            });
        return () => {
            ignore = true;
        };
    }, [mfaCode]);

    useEffect(() => {
        if (!selectedFile) return;
        let ignore = false;
        const reader = new FileReader();
        reader.onload = () => {
            if (!ignore && typeof reader.result === "string") setLocalPreview(reader.result);
        };
        reader.onerror = () => {
            if (!ignore) setLocalPreview(null);
        };
        reader.readAsDataURL(selectedFile);
        return () => {
            ignore = true;
            reader.abort();
        };
    }, [selectedFile]);

    function handleFileChange(event: ChangeEvent<HTMLInputElement>) {
        const file = event.target.files?.[0];
        if (!file) return;
        setError(null);
        setNotice(null);
        setLocalPreview(null);
        if (!["image/png", "image/jpeg"].includes(file.type)) {
            setSelectedFile(null);
            event.currentTarget.value = "";
            setError("請選擇 PNG 或 JPEG 圖片。");
            return;
        }
        if (file.size > maxImageBytes) {
            setSelectedFile(null);
            event.currentTarget.value = "";
            setError("圖片大小不可超過 5 MB。");
            return;
        }
        setSelectedFile(file);
    }

    async function handleUpload() {
        if (!selectedFile || saving) return;
        setSaving(true);
        setError(null);
        setNotice(null);
        const formData = new FormData();
        formData.append("file", selectedFile);
        try {
            const { data } = await uploadAdminArticleOverviewImage(formData, mfaCode || undefined);
            setMetadata(data);
            setHasCustomImage(true);
            setImageUrl(getArticleOverviewImageUrl(data.updated_at));
            setSelectedFile(null);
            setLocalPreview(null);
            if (fileInputRef.current) fileInputRef.current.value = "";
            setNotice("文章總覽主視覺已更新，前台重新整理後會套用新圖片。");
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setSaving(false);
        }
    }

    async function handleReset() {
        if (saving) return;
        setError(null);
        setNotice(null);
        if (!hasCustomImage) {
            setSelectedFile(null);
            setLocalPreview(null);
            if (fileInputRef.current) fileInputRef.current.value = "";
            setNotice("目前已使用預設圖片。");
            return;
        }

        setSaving(true);
        try {
            await resetAdminArticleOverviewImage(mfaCode || undefined);
            setMetadata({ exists: false });
            setHasCustomImage(false);
            setImageUrl(fallbackArticleOverviewImage);
            setSelectedFile(null);
            setLocalPreview(null);
            if (fileInputRef.current) fileInputRef.current.value = "";
            setNotice("已恢復文章總覽的預設圖片。");
        } catch (cause) {
            setError(describeError(cause));
        } finally {
            setSaving(false);
        }
    }

    const previewImage = localPreview ?? imageUrl;

    return (
        <AdminPanel className="sm:p-6">
            <div className="flex flex-wrap items-start justify-between gap-4">
                <div>
                    <h2 className="font-serif text-xl text-ink">廣告中心</h2>
                    <p className="mt-2 max-w-2xl font-sans text-sm leading-6 text-copy-muted">
                        目前用來管理文章總覽主視覺；未來可在此擴充廣告素材、版位與曝光設定。上傳後會儲存在後端，不需要再修改程式碼源碼。
                    </p>
                </div>
                <Badge tone={hasCustomImage ? "positive" : "neutral"}>
                    {hasCustomImage ? "已使用自訂圖片" : "使用預設圖片"}
                </Badge>
            </div>

            {loading ? (
                <LoadingState label="載入主視覺設定中…" />
            ) : (
                <div className="mt-5 grid min-w-0 gap-5 lg:grid-cols-[minmax(0,1.2fr)_minmax(18rem,0.8fr)] lg:items-start">
                    <div className="min-w-0">
                        <div className="relative aspect-[1891/831] w-full max-w-full overflow-hidden rounded-[15%] bg-surface">
                            <Image
                                src={previewImage}
                                alt="文章總覽主視覺預覽"
                                fill
                                unoptimized
                                sizes="(min-width: 1024px) 60vw, 100vw"
                                className="object-contain"
                            />
                        </div>
                        <p className="mt-2 font-sans text-xs text-copy-muted">
                            {selectedFile
                                ? "這是尚未儲存的本機預覽。"
                                : metadata?.updated_at
                                  ? "最後更新：" + formatDate(metadata.updated_at)
                                  : "目前顯示前端內建的預設圖片。"}
                        </p>
                    </div>

                    <div className="min-w-0 rounded-[var(--radius-small)] border border-ink/10 bg-ink/[0.02] p-4 sm:p-5">
                        <label
                            htmlFor="article-overview-image"
                            className="font-sans text-sm font-bold text-ink"
                        >
                            選擇替換圖片
                        </label>
                        <input
                            ref={fileInputRef}
                            id="article-overview-image"
                            type="file"
                            accept="image/png,image/jpeg"
                            onChange={handleFileChange}
                            className="mt-3 block w-full min-w-0 font-sans text-sm text-ink file:mr-3 file:rounded-full file:border-0 file:bg-ink file:px-4 file:py-2 file:font-sans file:text-sm file:font-bold file:text-surface hover:file:bg-ink/80"
                        />
                        <p className="mt-3 font-sans text-xs leading-5 text-copy-muted">
                            支援 PNG、JPEG，大小上限 5 MB。建議使用接近 1891 × 831
                            的寬幅圖片，手機畫面會自動縮放到可用寬度。
                        </p>
                        <div className="mt-5 flex flex-wrap justify-end gap-2">
                            <Button
                                type="button"
                                variant="secondary"
                                onClick={() => void handleReset()}
                                disabled={saving || (!hasCustomImage && !selectedFile)}
                                className="h-10 px-4 font-sans text-sm"
                            >
                                恢復預設
                            </Button>
                            <Button
                                type="button"
                                onClick={() => void handleUpload()}
                                disabled={saving || !selectedFile}
                                className="h-10 px-4 font-sans text-sm"
                            >
                                {saving ? "處理中…" : "上傳並套用"}
                            </Button>
                        </div>
                    </div>
                </div>
            )}

            {error ? (
                <div className="mt-4">
                    <ErrorText>{error}</ErrorText>
                </div>
            ) : null}
            {notice ? <p className="mt-4 font-sans text-sm text-green-700">{notice}</p> : null}
        </AdminPanel>
    );
}
