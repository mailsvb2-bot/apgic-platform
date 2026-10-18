import { expect, test } from "@playwright/test";

test("customer can enter real public specialist catalogue directly from homepage", async ({ page }) => {
  await page.route("**/v1/specialists?*", async route =>
    route.fulfill({status:200,contentType:"application/json",body:JSON.stringify({
      profiles:[],has_more:false,limit:10,offset:0,sort:"DISPLAY_NAME_ASC",
    })})
  );
  await page.goto("/");
  const directoryLink = page.getByRole("link", {name:"Смотреть каталог специалистов"});
  await expect(directoryLink).toHaveAttribute("href","/specialists");
  await directoryLink.click();
  await expect(page).toHaveURL(/\/specialists\/?$/);
  await expect(page.getByRole("heading",{name:"Выберите специалиста"})).toBeVisible();
  // A public visitor must not be shown invented specialists when the API has none.
  await expect(page.getByText("Сейчас нет опубликованных проверенных анкет")).toBeVisible();
});
