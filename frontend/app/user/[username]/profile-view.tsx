"use client";

import Link from "next/link";
import { BadgeCheck, ExternalLink, GraduationCap } from "lucide-react";
import { useCallback } from "react";
import { EmptyState, Resource, StatusBadge } from "../../components/status";
import Markdown from "../../components/markdown";
import Avatar from "../../components/avatar";
import Button from "../../components/button";
import { Card, ContentCard } from "../../components/card";
import HashtagBadge from "../../components/hashtag-badge";
import StatusPage from "../../components/status-page";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../../components/tabs";
import { readingTimeOf } from "../../article/article-format";
import { formatNumericDate } from "../../lib/format";
import { listUserExperiences, type TaggedExperience } from "../../lib/api/content";
import {
    displayNameOf,
    getPublicProfile,
    getPublicProfileDetails,
    identityStatusLabels,
    listUserPortfolio,
    profileAvatarSrc,
    resolveUser,
    type Profile,
    type PublicPortfolioItem,
    type PublicProfileDetails
} from "../../lib/api/profile";
import { ApiError } from "../../lib/api/types";
import { experienceHref, loginHref, userHref } from "../../lib/routes";
import { useResource } from "../../lib/use-resource";

type ProfilePage = {
    profile: Profile;
    details: PublicProfileDetails;
    articles: TaggedExperience[];
    portfolio: PublicPortfolioItem[];
};

const isStatus = (cause: unknown, status: number) =>
    cause instanceof ApiError && cause.status === status;

/** `segment` is the URL's /user/{segment}: a username or a custom slug. */
export default function ProfileView({ username: segment }: { username: string }) {
    const load = useCallback(
        async (signal: AbortSignal): Promise<ProfilePage> => {
            const {
                data: { username }
            } = await resolveUser(segment, { signal });
            const [profile, details, articles, portfolio] = await Promise.all([
                getPublicProfile(username, { signal }),
                getPublicProfileDetails(username, { signal }),
                listUserExperiences(username, { signal }),
                listUserPortfolio(username, { signal })
            ]);
            return {
                profile: profile.data,
                details: details.data,
                articles: articles.data,
                portfolio: portfolio.data
            };
        },
        [segment]
    );
    const { state, reload } = useResource(load);

    if (state.status === "error" && isStatus(state.error, 404)) {
        return (
            <StatusPage
                eyebrow="404"
                title="找不到這位使用者"
                description="這個個人頁面不存在，可能是網址打錯了，或是帳號已經不再公開。"
            />
        );
    }

    return (
        <main className="article-dots flex-1 bg-surface">
            <div className="mx-auto w-full max-w-screen-lg px-5 py-10 sm:px-6 sm:py-14 lg:px-16">
                <Resource
                    state={state}
                    reload={reload}
                    loadingLabel="正在載入個人頁面"
                    renderError={(error) =>
                        isStatus(error, 401) ? (
                            <EmptyState
                                title="登入後即可查看個人頁面"
                                description="目前個人頁面只開放給已登入的會員瀏覽。"
                                action={
                                    <Button asChild>
                                        <Link href={loginHref(userHref(segment))}>登入</Link>
                                    </Button>
                                }
                            />
                        ) : undefined
                    }
                >
                    {(page) => <ProfileContent {...page} />}
                </Resource>
            </div>
        </main>
    );
}

function ProfileContent({ profile, details, articles, portfolio }: ProfilePage) {
    const name = displayNameOf(profile);
    const identity =
        profile.identity_status === "temporary"
            ? null
            : identityStatusLabels[profile.identity_status];
    const affiliation = [details.school_name, details.department].filter(Boolean).join("・");
    const links = profile.links.filter((link) => /^https?:\/\//i.test(link.url));

    return (
        <>
            <header className="flex flex-col items-center gap-5 text-center sm:flex-row sm:items-center sm:gap-8 sm:text-left">
                <Avatar
                    name={name}
                    src={profileAvatarSrc(profile)}
                    size="lg"
                    className="sm:size-36 sm:text-5xl"
                />
                <div className="flex min-w-0 flex-col items-center gap-2 sm:items-start">
                    <h1 className="max-w-full font-sans text-3xl leading-tight font-medium tracking-[-0.02em] break-words text-ink sm:text-4xl">
                        {name}
                    </h1>
                    <p className="font-sans text-base text-copy-muted">@{profile.username}</p>
                    <div className="flex flex-wrap items-center justify-center gap-x-4 gap-y-2 font-sans text-sm text-ink sm:justify-start">
                        {identity && (
                            <StatusBadge tone="success" className="gap-1.5">
                                <BadgeCheck aria-hidden className="size-4" />
                                {identity}
                            </StatusBadge>
                        )}
                        {affiliation && (
                            <span className="inline-flex items-center gap-1.5 text-copy-muted">
                                <GraduationCap aria-hidden className="size-4" />
                                {affiliation}
                            </span>
                        )}
                    </div>
                </div>
            </header>

            <Tabs defaultValue="bio" className="mt-10 sm:mt-12">
                <TabsList aria-label="個人頁面內容">
                    <TabsTrigger value="bio">個人簡介</TabsTrigger>
                    {details.show_articles && (
                        <TabsTrigger value="articles">發布的文章</TabsTrigger>
                    )}
                    {details.show_portfolio && (
                        <TabsTrigger value="portfolio">公開備審</TabsTrigger>
                    )}
                </TabsList>

                <TabsContent value="bio" className="mt-6">
                    <Card className="flex flex-col gap-6">
                        {profile.bio?.trim() ? (
                            <Markdown markdown={profile.bio} />
                        ) : (
                            <p className="font-sans text-base text-copy-muted">
                                這位使用者還沒有填寫個人簡介。
                            </p>
                        )}
                        {links.length > 0 && (
                            <ul aria-label="個人連結" className="flex flex-wrap gap-2">
                                {links.map((link) => (
                                    <li key={link.url} className="min-w-0">
                                        <a
                                            href={link.url}
                                            target="_blank"
                                            rel="noopener noreferrer nofollow"
                                            className="inline-flex max-w-full items-center gap-2 rounded-full border border-ink/15 px-3 py-1.5 font-sans text-sm text-ink focus-ring transition-colors hover:bg-ink/5"
                                        >
                                            <ExternalLink aria-hidden className="size-4 shrink-0" />
                                            <span className="truncate">
                                                {link.label || link.url}
                                            </span>
                                        </a>
                                    </li>
                                ))}
                            </ul>
                        )}
                    </Card>
                </TabsContent>

                {details.show_articles && (
                    <TabsContent value="articles" className="mt-6">
                        {articles.length > 0 ? (
                            <ul className="flex flex-col gap-4">
                                {articles.map((article) => (
                                    <li key={article.id}>
                                        <ContentCard
                                            href={experienceHref(article.id)}
                                            title={article.title}
                                            excerpt={article.body.slice(0, 160)}
                                            thumbnailSeed={article.id}
                                            meta={
                                                <>
                                                    <time dateTime={article.created_at}>
                                                        {formatNumericDate(article.created_at)}
                                                    </time>
                                                    <span>{readingTimeOf(article.body)}</span>
                                                    {article.tags.map((tag) => (
                                                        <HashtagBadge key={tag} tag={tag} />
                                                    ))}
                                                </>
                                            }
                                        />
                                    </li>
                                ))}
                            </ul>
                        ) : (
                            <EmptyState title="還沒有發布任何文章" />
                        )}
                    </TabsContent>
                )}

                {details.show_portfolio && (
                    <TabsContent value="portfolio" className="mt-6">
                        {portfolio.length > 0 ? (
                            <ul className="grid gap-4 sm:grid-cols-2">
                                {portfolio.map((item) => (
                                    <li key={item.file_id}>
                                        <Card className="h-full">
                                            <p className="font-sans text-sm text-copy-muted">
                                                {item.academic_year} 學年度・{item.school_name}
                                            </p>
                                            <h3 className="mt-1 font-sans text-xl font-medium text-ink">
                                                {item.project_title}
                                            </h3>
                                            <p className="mt-2 font-sans text-sm text-copy-muted">
                                                {item.program_name}
                                            </p>
                                        </Card>
                                    </li>
                                ))}
                            </ul>
                        ) : (
                            <EmptyState title="還沒有公開的備審資料" />
                        )}
                    </TabsContent>
                )}
            </Tabs>
        </>
    );
}
