package com.saby.personalportfolio

import android.os.Bundle
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge

class TagsActivity : AppCompatActivity() {

    private val gson = Gson()
    private lateinit var existingCaption: TextView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_tags)

        existingCaption = findViewById(R.id.tagsExistingCaption)
        val recyclerView = findViewById<RecyclerView>(R.id.tagsRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)
        loadAndBindAdapter(recyclerView)
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun loadAndBindAdapter(recyclerView: RecyclerView) {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)

        val snapshot: PortfolioAssetsSnapshot = try {
            gson.fromJson(portfolioJson, PortfolioAssetsSnapshot::class.java)
        } catch (e: Exception) {
            Toast.makeText(this, "Could not read portfolio: ${e.message}", Toast.LENGTH_LONG).show()
            PortfolioAssetsSnapshot(emptyList(), emptyList(), emptyList())
        }

        val assets = snapshot.assets.orEmpty().sortedBy { FundNameFormatter.shorten(it.name) }
        recyclerView.adapter = TagsAdapter(assets) { assetId, tags, primaryTag, rowHolder ->
            saveTags(assetId, tags, primaryTag, rowHolder)
        }

        showExistingTagsCaption(portfolioJson)
    }

    private fun showExistingTagsCaption(portfolioJson: String) {
        val allTagsJson = Bridge.computeAllTags(portfolioJson)
        if (isBridgeError(allTagsJson)) return // caption is a convenience, not worth surfacing an error dialog for
        val tagListType = object : TypeToken<List<String>>() {}.type
        val allTags: List<String> = try {
            gson.fromJson(allTagsJson, tagListType) ?: emptyList()
        } catch (e: Exception) {
            emptyList()
        }
        existingCaption.text = if (allTags.isEmpty()) {
            "Existing tags: none yet"
        } else {
            "Existing tags: " + allTags.joinToString(", ")
        }
    }

    private fun saveTags(assetId: String, tags: List<String>, primaryTag: String, rowHolder: TagsAdapter.RowHolder) {
        rowHolder.saveButton.isEnabled = false
        val portfolioPath = PortfolioStorage.filePath(this)
        val currentPortfolioJson = PortfolioLoadCache.load(portfolioPath)
        if (isBridgeError(currentPortfolioJson)) {
            rowHolder.saveButton.isEnabled = true
            Toast.makeText(this, "Failed to load portfolio: $currentPortfolioJson", Toast.LENGTH_LONG).show()
            return
        }

        val tagsJson = gson.toJson(tags)
        val afterTags = Bridge.setAssetTags(currentPortfolioJson, assetId, tagsJson)
        if (isBridgeError(afterTags)) {
            rowHolder.saveButton.isEnabled = true
            Toast.makeText(this, "Failed to set tags: $afterTags", Toast.LENGTH_LONG).show()
            return
        }

        val afterPrimary = Bridge.setAssetPrimaryTag(afterTags, assetId, primaryTag)
        if (isBridgeError(afterPrimary)) {
            rowHolder.saveButton.isEnabled = true
            Toast.makeText(this, "Failed to set primary tag: $afterPrimary", Toast.LENGTH_LONG).show()
            return
        }

        val saveResult = Bridge.savePortfolio(portfolioPath, afterPrimary)
        rowHolder.saveButton.isEnabled = true
        if (isBridgeError(saveResult)) {
            Toast.makeText(this, "Failed to save: $saveResult", Toast.LENGTH_LONG).show()
            return
        }
        showExistingTagsCaption(afterPrimary)
        Toast.makeText(this, if (tags.isEmpty()) "Untagged" else "Saved: ${tags.joinToString(", ")}", Toast.LENGTH_SHORT).show()
    }
}
