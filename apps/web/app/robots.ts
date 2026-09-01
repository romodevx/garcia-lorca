import type { MetadataRoute } from "next";

/**
 * Keep the demo out of search results.
 *
 * The web service is invokable by allUsers — it has to be, or the school could
 * not open the link themselves — so the login page answers 200 to anyone who has
 * the URL. The pupil data behind it is safe (no session, 401), but the door is
 * visible, and a link pasted into a chat or an email can end up crawled.
 *
 * This is not a security control: `roles/run.invoker` on allUsers and the
 * gateway's auth are. It is hygiene, so a demo environment holding a school's
 * name never turns up in a search result.
 */
export default function robots(): MetadataRoute.Robots {
  return {
    rules: {
      userAgent: "*",
      disallow: "/",
    },
  };
}
