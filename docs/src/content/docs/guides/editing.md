---
title: Editing Data
description: How to edit artist, album, and track information in your Koito instance.
---

In order to start editing information on your Koito instance, you need to be logged in. See the [Setting up the Scrobber](/guides/scrobbler) guide if you need to log in for the first time.

Once logged in, navigate to the page of the item you want to edit. For this example, we will use the fantastic Korean dream pop group OurR's track [Bloom](https://www.youtube.com/watch?v=USHrBJRmF-o).
When you are logged in and on an artist, album, or track page, you will see the editing options on the top right.

![A Koito page for the track Bloom by OurR, showing the editing options](../../../assets/ourr_bloom.png)

#### Editing

The first option from the left is the edit menu. The edit menu lets you edit things like aliases, artists, and MusicBrainz IDs.

![a menu showing editing options](../../../assets/edit_menu.png)

In the alias manager section, we can add and remove aliases, as well as set an alias as "primary" to use it as the display name of the item. Koito uses aliases for not only the UI, but also for matching artists, albums, and tracks submitted to the scrobbler, and for searching. So if you are like me and listen to a lot of music from countries with non-latin script, adding aliases makes it easy to search for those items.

We can also add and remove artist associations, as well as update the MusicBrainz ID of the item. Note that MusicBrainz IDs for items must be unique among other items of the same type (artists, albums, and tracks).

For artists and albums, similar editing options will be shown.

#### Editing Images

The easiest way to replace an image is to simply drag an image file from your computer onto the page of the artist, album, or track you want to change the image for. This only works when you are logged in.

The other way to replace images is using the Replace Image option, which is the second editing option in the list, to provide a URL to the image you want to use.

Note that you can only edit images on an artist or album page, as the image shown for a track is the same as the image of the album it belongs to.

#### Matching

This fork replaces destructive merges with reversible matching rules.
See the [Filters design](https://github.com/ephun/Koito/blob/custom/koito/docs/filters-interface.md) for the planned behavior.

#### Deleting Items

To delete at item, just click the trash icon, in the editing options. Doing so will open a confirmation dialogue. Once confirmed, the item you delete, as well as all of its children and listen activity, will be removed.

:::danger

Deleting items is irreversible.

:::
