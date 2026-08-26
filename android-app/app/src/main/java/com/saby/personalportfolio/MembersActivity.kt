package com.saby.personalportfolio

import android.os.Bundle
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge

class MembersActivity : AppCompatActivity() {

    private val gson = Gson()
    private lateinit var recyclerView: RecyclerView
    private lateinit var statusText: TextView
    private lateinit var nameInput: EditText

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_members)

        recyclerView = findViewById(R.id.membersRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)
        statusText = findViewById(R.id.membersStatusText)
        nameInput = findViewById(R.id.newMemberNameInput)

        findViewById<Button>(R.id.addMemberButton).setOnClickListener { addMember() }
    }

    override fun onResume() {
        super.onResume()
        loadMembers()
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun loadMembers() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val membersJson = Bridge.listMembers(portfolioJson)

        val memberType = object : TypeToken<List<Member>>() {}.type
        val members: List<Member> = try {
            gson.fromJson(membersJson, memberType) ?: emptyList()
        } catch (e: Exception) {
            emptyList()
        }

        recyclerView.adapter = MembersAdapter(members)
    }

    private fun addMember() {
        val name = nameInput.text.toString().trim()
        if (name.isEmpty()) {
            statusText.text = "Enter a name first."
            return
        }

        val portfolioPath = PortfolioStorage.filePath(this)
        val currentPortfolioJson = PortfolioLoadCache.load(portfolioPath)
        if (isBridgeError(currentPortfolioJson)) {
            statusText.text = "Failed to load portfolio: $currentPortfolioJson"
            return
        }

        val updatedJson = Bridge.addMember(currentPortfolioJson, name)
        if (isBridgeError(updatedJson)) {
            statusText.text = updatedJson
            return
        }

        val saveResult = Bridge.savePortfolio(portfolioPath, updatedJson)
        if (isBridgeError(saveResult)) {
            statusText.text = "Failed to save: $saveResult"
            return
        }

        statusText.text = ""
        nameInput.text.clear()
        Toast.makeText(this, "Added $name", Toast.LENGTH_SHORT).show()
        loadMembers()
    }
}
