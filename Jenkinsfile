GKERegion='us-central1-a'

void CreateCluster(String CLUSTER_SUFFIX) {
    withCredentials([string(credentialsId: 'GCP_PROJECT_ID', variable: 'GCP_PROJECT'), file(credentialsId: 'gcloud-key-file', variable: 'CLIENT_SECRET_FILE')]) {
        sh """
            NODES_NUM=3
            export KUBECONFIG=/tmp/$CLUSTER_NAME-${CLUSTER_SUFFIX}
            export USE_GKE_GCLOUD_AUTH_PLUGIN=True
            source $HOME/google-cloud-sdk/path.bash.inc
            ret_num=0
            while [ \${ret_num} -lt 15 ]; do
                ret_val=0
                gcloud auth activate-service-account --key-file $CLIENT_SECRET_FILE
                gcloud config set project $GCP_PROJECT
                gcloud container clusters list --filter $CLUSTER_NAME-${CLUSTER_SUFFIX} --zone $GKERegion --format='csv[no-heading](name)' | xargs gcloud container clusters delete --zone $GKERegion --quiet || true
                gcloud container clusters create --zone $GKERegion $CLUSTER_NAME-${CLUSTER_SUFFIX} --cluster-version=1.21 --machine-type=n1-standard-4 --preemptible --num-nodes=\$NODES_NUM --network=jenkins-vpc --subnetwork=jenkins-${CLUSTER_SUFFIX} --no-enable-autoupgrade --cluster-ipv4-cidr=10.\$(( RANDOM % 250 )).\$(( RANDOM % 30 * 8 )).0/21 && \
                kubectl create clusterrolebinding cluster-admin-binding --clusterrole cluster-admin --user jenkins@"$GCP_PROJECT".iam.gserviceaccount.com || ret_val=\$?
                if [ \${ret_val} -eq 0 ]; then break; fi
                ret_num=\$((ret_num + 1))
            done
            if [ \${ret_num} -eq 15 ]; then exit 1; fi
        """
   }
}
void ShutdownCluster(String CLUSTER_SUFFIX) {
    withCredentials([string(credentialsId: 'GCP_PROJECT_ID', variable: 'GCP_PROJECT'), file(credentialsId: 'gcloud-key-file', variable: 'CLIENT_SECRET_FILE')]) {
        sh """
            export KUBECONFIG=/tmp/$CLUSTER_NAME-${CLUSTER_SUFFIX}
            export USE_GKE_GCLOUD_AUTH_PLUGIN=True
            source $HOME/google-cloud-sdk/path.bash.inc
            gcloud auth activate-service-account --key-file $CLIENT_SECRET_FILE
            gcloud config set project $GCP_PROJECT
            gcloud container clusters delete --zone $GKERegion $CLUSTER_NAME-${CLUSTER_SUFFIX}
        """
   }
}
void pushLogFile(String FILE_NAME) {
    LOG_FILE_PATH="e2e-tests/logs/${FILE_NAME}.log"
    LOG_FILE_NAME="${FILE_NAME}.log"
    echo "Push logfile $LOG_FILE_NAME file to S3!"
    withCredentials([[$class: 'AmazonWebServicesCredentialsBinding', accessKeyVariable: 'AWS_ACCESS_KEY_ID', credentialsId: 'AMI/OVF', secretKeyVariable: 'AWS_SECRET_ACCESS_KEY']]) {
        sh """
            S3_PATH=s3://percona-jenkins-artifactory-public/\$JOB_NAME/\$(git rev-parse --short HEAD)
            aws s3 ls \$S3_PATH/${LOG_FILE_NAME} || :
            aws s3 cp --content-type text/plain --quiet ${LOG_FILE_PATH} \$S3_PATH/${LOG_FILE_NAME} || :
        """
    }
}
void pushArtifactFile(String FILE_NAME) {
    echo "Push $FILE_NAME file to S3!"

    withCredentials([[$class: 'AmazonWebServicesCredentialsBinding', accessKeyVariable: 'AWS_ACCESS_KEY_ID', credentialsId: 'AMI/OVF', secretKeyVariable: 'AWS_SECRET_ACCESS_KEY']]) {
        sh """
            touch ${FILE_NAME}
            S3_PATH=s3://percona-jenkins-artifactory/\$JOB_NAME/\$(git rev-parse --short HEAD)
            aws s3 ls \$S3_PATH/${FILE_NAME} || :
            aws s3 cp --quiet ${FILE_NAME} \$S3_PATH/${FILE_NAME} || :
        """
    }
}

void popArtifactFile(String FILE_NAME) {
    echo "Try to get $FILE_NAME file from S3!"

    withCredentials([[$class: 'AmazonWebServicesCredentialsBinding', accessKeyVariable: 'AWS_ACCESS_KEY_ID', credentialsId: 'AMI/OVF', secretKeyVariable: 'AWS_SECRET_ACCESS_KEY']]) {
        sh """
            S3_PATH=s3://percona-jenkins-artifactory/\$JOB_NAME/\$(git rev-parse --short HEAD)
            aws s3 cp --quiet \$S3_PATH/${FILE_NAME} ${FILE_NAME} || :
        """
    }
}

void printKubernetesStatus(String LOCATION, String CLUSTER_SUFFIX) {
    sh """
		export KUBECONFIG=/tmp/$CLUSTER_NAME-$CLUSTER_SUFFIX
		export USE_GKE_GCLOUD_AUTH_PLUGIN=True
		source $HOME/google-cloud-sdk/path.bash.inc
        echo "========== KUBERNETES STATUS $LOCATION TEST =========="
        gcloud container clusters list|grep -E "NAME|$CLUSTER_NAME-$CLUSTER_SUFFIX "
        echo
        kubectl get nodes
        echo
        kubectl top nodes
        echo
        kubectl get pods --all-namespaces
        echo
        kubectl top pod --all-namespaces
        echo
        kubectl get events --field-selector type!=Normal --all-namespaces
        echo "======================================================"
    """
}

TestsReport = '| Test name  | Status |\r\n| ------------- | ------------- |'
testsReportMap  = [:]
testsResultsMap = [:]

void makeReport() {
    def wholeTestAmount=sh(script: 'grep "runTest(.*)$" Jenkinsfile | grep -v wholeTestAmount | wc -l', , returnStdout: true).trim().toInteger()
    def startedTestAmount = testsReportMap.size()
    
    for ( test in testsReportMap.sort() ) {
        TestsReport = TestsReport + "\r\n| ${test.key} | ${test.value} |"
    }
    TestsReport = TestsReport + "\r\n| We run $startedTestAmount out of $wholeTestAmount|"
}

void setTestsresults() {
    testsResultsMap.each { file ->
        pushArtifactFile("${file.key}")
    }
}

void runTest(String TEST_NAME, String CLUSTER_SUFFIX, String MYSQL_VERSION, Integer TIMEOUT) {
    def retryCount = 0
    def testNameWithMysqlVersion = "$TEST_NAME-$MYSQL_VERSION".replace(".", "-")
    waitUntil {
        def testUrl = "https://percona-jenkins-artifactory-public.s3.amazonaws.com/cloud-pxc-operator/${env.GIT_BRANCH}/${env.GIT_SHORT_COMMIT}/${testNameWithMysqlVersion}.log"
        echo " test url is $testUrl"
        try {
            echo "The $TEST_NAME test was started!"
            testsReportMap["$testNameWithMysqlVersion"] = "[failed]($testUrl)"
            popArtifactFile("${env.GIT_BRANCH}-${env.GIT_SHORT_COMMIT}-$testNameWithMysqlVersion")

            timeout(time: TIMEOUT, unit: 'MINUTES') {
                sh """
                    if [ -f "${env.GIT_BRANCH}-${env.GIT_SHORT_COMMIT}-$testNameWithMysqlVersion" ]; then
                        echo Skip $TEST_NAME test
                    else
                        export KUBECONFIG=/tmp/$CLUSTER_NAME-${CLUSTER_SUFFIX}
                        export MYSQL_VERSION=$MYSQL_VERSION
                        source $HOME/google-cloud-sdk/path.bash.inc
                        time bash ./e2e-tests/$TEST_NAME/run
                    fi
                """
            }
            echo "end test url is $testUrl"
            testsReportMap["$testNameWithMysqlVersion"] = "[passed]($testUrl)"
            testsResultsMap["${env.GIT_BRANCH}-${env.GIT_SHORT_COMMIT}-$testNameWithMysqlVersion"] = 'passed'
            return true
        }
        catch (exc) {
            printKubernetesStatus("AFTER","$CLUSTER_SUFFIX")
            if (retryCount >= 1) {
                currentBuild.result = 'FAILURE'
                return true
            }
            retryCount++
            return false
        }
        finally {
            pushLogFile("$testNameWithMysqlVersion")
            echo "The $TEST_NAME test was finished!"
        }
    }
}

void installRpms() {
    sh '''
        sudo yum install -y https://repo.percona.com/yum/percona-release-latest.noarch.rpm || true
        sudo percona-release enable-only tools
        sudo yum install -y percona-xtrabackup-80 jq | true
    '''
}

def skipBranchBuilds = true
if ( env.CHANGE_URL ) {
    skipBranchBuilds = false
}

pipeline {
    environment {
        CLOUDSDK_CORE_DISABLE_PROMPTS = 1
        CLEAN_NAMESPACE = 1
        OPERATOR_NS = 'pxc-operator'
        GIT_SHORT_COMMIT = sh(script: 'git describe --always --dirty', , returnStdout: true).trim()
        VERSION = "${env.GIT_BRANCH}-${env.GIT_SHORT_COMMIT}"
        CLUSTER_NAME = sh(script: "echo jenkins-pxc-${GIT_SHORT_COMMIT} | tr '[:upper:]' '[:lower:]'", , returnStdout: true).trim()
        AUTHOR_NAME  = sh(script: "echo ${CHANGE_AUTHOR_EMAIL} | awk -F'@' '{print \$1}'", , returnStdout: true).trim()
        ENABLE_LOGGING="true"
    }
    agent {
        label 'docker'
    }
    stages {
        stage('Prepare') {
            when {
                expression {
                    !skipBranchBuilds
                }
            }
            steps {
                installRpms()
                script {
                    if ( AUTHOR_NAME == 'null' )  {
                        AUTHOR_NAME = sh(script: "git show -s --pretty=%ae | awk -F'@' '{print \$1}'", , returnStdout: true).trim()
                    }
                    for (comment in pullRequest.comments) {
                        println("Author: ${comment.user}, Comment: ${comment.body}")
                        if (comment.user.equals('JNKPercona')) {
                            println("delete comment")
                            comment.delete()
                        }
                    }
                }
                sh '''
                    if [ ! -d $HOME/google-cloud-sdk/bin ]; then
                        rm -rf $HOME/google-cloud-sdk
                        curl https://sdk.cloud.google.com | bash
                    fi

                    source $HOME/google-cloud-sdk/path.bash.inc
                    gcloud components install alpha
                    gcloud components install kubectl
                    curl -fsSL https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash
                    curl -s -L https://github.com/openshift/origin/releases/download/v3.11.0/openshift-origin-client-tools-v3.11.0-0cbc58b-linux-64bit.tar.gz \
                        | sudo tar -C /usr/local/bin --strip-components 1 --wildcards -zxvpf - '*/oc'

                    curl -s -L https://github.com/mitchellh/golicense/releases/latest/download/golicense_0.2.0_linux_x86_64.tar.gz \
                        | sudo tar -C /usr/local/bin --wildcards -zxvpf -

                    sudo sh -c "curl -s -L https://github.com/mikefarah/yq/releases/download/v4.27.2/yq_linux_amd64 > /usr/local/bin/yq"
                    sudo chmod +x /usr/local/bin/yq
                '''
                withCredentials([file(credentialsId: 'cloud-secret-file', variable: 'CLOUD_SECRET_FILE')]) {
                    sh '''
                        cp $CLOUD_SECRET_FILE ./e2e-tests/conf/cloud-secret.yml
                    '''
                }
            }
        }
        stage('Build docker image') {
            when {
                expression {
                    !skipBranchBuilds
                }
            }
            steps {
                withCredentials([usernamePassword(credentialsId: 'hub.docker.com', passwordVariable: 'PASS', usernameVariable: 'USER')]) {
                    sh '''
                        DOCKER_TAG=perconalab/percona-xtradb-cluster-operator:$VERSION
                        docker_tag_file='./results/docker/TAG'
                        mkdir -p $(dirname ${docker_tag_file})
                        echo ${DOCKER_TAG} > "${docker_tag_file}"
                            sg docker -c "
                                docker login -u '${USER}' -p '${PASS}'
                                export RELEASE=0
                                export IMAGE=\$DOCKER_TAG
                                ./e2e-tests/build
                                docker logout
                            "
                        sudo rm -rf ./build
                    '''
                }
                stash includes: 'results/docker/TAG', name: 'IMAGE'
                archiveArtifacts 'results/docker/TAG'
            }
        }
        stage('GoLicenseDetector test') {
            when {
                expression {
                    !skipBranchBuilds
                }
            }
            steps {
                sh """
                    mkdir -p $WORKSPACE/src/github.com/percona
                    ln -s $WORKSPACE $WORKSPACE/src/github.com/percona/percona-xtradb-cluster-operator
                    sg docker -c "
                        docker run \
                            --rm \
                            -v $WORKSPACE/src/github.com/percona/percona-xtradb-cluster-operator:/go/src/github.com/percona/percona-xtradb-cluster-operator \
                            -w /go/src/github.com/percona/percona-xtradb-cluster-operator \
                            golang:1.19 sh -c '
                                go install -mod=readonly github.com/google/go-licenses@latest;
                                /go/bin/go-licenses csv github.com/percona/percona-xtradb-cluster-operator/cmd/manager \
                                    | cut -d , -f 3 \
                                    | sort -u \
                                    > go-licenses-new || :
                            '
                    "
                    diff -u e2e-tests/license/compare/go-licenses go-licenses-new
                """
            }
        }
        stage('GoLicense test') {
            when {
                expression {
                    !skipBranchBuilds
                }
            }
            steps {
                sh '''
                    mkdir -p $WORKSPACE/src/github.com/percona
                    ln -s $WORKSPACE $WORKSPACE/src/github.com/percona/percona-xtradb-cluster-operator
                    sg docker -c "
                        docker run \
                            --rm \
                            -v $WORKSPACE/src/github.com/percona/percona-xtradb-cluster-operator:/go/src/github.com/percona/percona-xtradb-cluster-operator \
                            -w /go/src/github.com/percona/percona-xtradb-cluster-operator \
                            -e GO111MODULE=on \
                            golang:1.19 sh -c 'go build -v -o percona-xtradb-cluster-operator github.com/percona/percona-xtradb-cluster-operator/cmd/manager'
                    "
                '''

                withCredentials([string(credentialsId: 'GITHUB_API_TOKEN', variable: 'GITHUB_TOKEN')]) {
                    sh """
                        golicense -plain ./percona-xtradb-cluster-operator \
                            | grep -v 'license not found' \
                            | sed -r 's/^[^ ]+[ ]+//' \
                            | sort \
                            | uniq \
                            > golicense-new || true
                        diff -u e2e-tests/license/compare/golicense golicense-new
                    """
                }
            }
        }
        stage('Run tests for operator') {
            when {
                expression {
                    !skipBranchBuilds
                }
            }
            options {
                timeout(time: 3, unit: 'HOURS')
            }
            parallel {
                stage('cluster1') {
                    steps {
                        CreateCluster('cluster1')
                        runTest('upgrade-haproxy', 'cluster1', '8.0', 45)
                        runTest('upgrade-proxysql', 'cluster1', '8.0', 45)
                        ShutdownCluster('cluster1')
                    }
                }
                stage('cluster2') {
                    steps {
                        CreateCluster('cluster2')
                        runTest('smart-update1', 'cluster2', '8.0', 75)
                        runTest('smart-update2', 'cluster2', '8.0', 45)
                        ShutdownCluster('cluster2')
                    }
                }
                stage('cluster3') {
                    steps {
                        CreateCluster('cluster3')
                        runTest('init-deploy', 'cluster3', '8.0', 40)
                        runTest('limits', 'cluster3', '8.0', 30)
                        runTest('monitoring-2-0', 'cluster3', '8.0', 30)
                        ShutdownCluster('cluster3')
                    }
                }
                stage('cluster4') {
                    steps {
                        CreateCluster('cluster4')
                        runTest('proxysql-sidecar-res-limits', 'cluster4', '8.0', 30)
                        runTest('tls-issue-self','cluster4', '8.0', 25)
                        runTest('tls-issue-cert-manager','cluster4', '8.0', 25)
                        runTest('tls-issue-cert-manager-ref','cluster4', '8.0', 25)
                        runTest('validation-hook','cluster4', '8.0', 10)
                        ShutdownCluster('cluster4')
                    }
                }
                stage('cluster5') {
                    steps {
                        CreateCluster('cluster5')
                        runTest('scaling', 'cluster5', '8.0', 30)
                        runTest('scaling-proxysql', 'cluster5', '8.0', 30)
                        runTest('security-context', 'cluster5', '8.0', 50)
                        ShutdownCluster('cluster5')
                    }
                }
                stage('cluster6') {
                    steps {
                        CreateCluster('cluster6')
                        runTest('storage', 'cluster6', '8.0', 30)
                        runTest('upgrade-consistency', 'cluster6', '8.0', 50)
                        runTest('proxy-protocol','cluster6', '8.0', 30)
                        ShutdownCluster('cluster6')
                    }
                }
                stage('cluster7') {
                    steps {
                        CreateCluster('cluster7')
                        runTest('restore-to-encrypted-cluster', 'cluster7', '8.0', 50)
                        runTest('pitr', 'cluster7', '8.0', 75)
                        runTest('affinity', 'cluster7', '8.0', 20)
                        ShutdownCluster('cluster7')
                    }
                }
                stage('cluster8') {
                    steps {
                        CreateCluster('cluster8')
                        runTest('scheduled-backup', 'cluster8', '8.0', 60)
                        ShutdownCluster('cluster8')
                    }
                }
                stage('cluster9') {
                    steps {
                        CreateCluster('cluster9')
                        runTest('cross-site', 'cluster9', '8.0', 50)
                        runTest('recreate', 'cluster9', '8.0', 50)
                        ShutdownCluster('cluster9')
                    }
                }
                stage('cluster10') {
                    steps {
                        CreateCluster('cluster10')
                        runTest('users', 'cluster10', '8.0', 75)
                        ShutdownCluster('cluster10')
                    }
                }
                stage('cluster11') {
                    steps {
                        CreateCluster('cluster11')
                        runTest('demand-backup', 'cluster11', '8.0', 60)
                        runTest('demand-backup-cloud', 'cluster11', '8.0', 60)
                        ShutdownCluster('cluster11')
                    }
                }
                stage('cluster12') {
                    steps {
                        CreateCluster('cluster12')
                        runTest('demand-backup-encrypted-with-tls', 'cluster12', '8.0', 75)
                        ShutdownCluster('cluster12')
                    }
                }
                stage('cluster13') {
                    steps {
                        CreateCluster('cluster13')
                        runTest('haproxy', 'cluster13', '8.0', 50)
                        runTest('one-pod', 'cluster13', '8.0', 30)
                        runTest('auto-tuning', 'cluster13', '8.0', 30)
                        ShutdownCluster('cluster13')
                    }
                }
                stage('cluster14') {
                    steps {
                        CreateCluster('cluster14')
                        runTest('users', 'cluster14', '5.7', 75)
                        runTest('one-pod', 'cluster14', '5.7', 30)
                        ShutdownCluster('cluster14')
                    }
                }
                stage('cluster15') {
                    steps {
                        CreateCluster('cluster15')
                        runTest('scheduled-backup', 'cluster15', '5.7', 60)
                        runTest('init-deploy', 'cluster15', '5.7', 40)
                        runTest('haproxy', 'cluster15', '5.7', 50)
                        ShutdownCluster('cluster15')
                    }
                }
            }
        }
    }
    post {
        always {
            script {
                setTestsresults()
                if (currentBuild.result != null && currentBuild.result != 'SUCCESS') {

                    try {
                        slackSend channel: "@${AUTHOR_NAME}", color: '#FF0000', message: "[${JOB_NAME}]: build ${currentBuild.result}, ${BUILD_URL} owner: @${AUTHOR_NAME}"
                    }
                    catch (exc) {
                        slackSend channel: '#cloud-dev-ci', color: '#FF0000', message: "[${JOB_NAME}]: build ${currentBuild.result}, ${BUILD_URL} owner: @${AUTHOR_NAME}"
                    }
                }
                if (env.CHANGE_URL) {
                    for (comment in pullRequest.comments) {
                        println("Author: ${comment.user}, Comment: ${comment.body}")
                        if (comment.user.equals('JNKPercona')) {
                            println("delete comment")
                            comment.delete()
                        }
                    }
                    makeReport()
                    unstash 'IMAGE'
                    def IMAGE = sh(returnStdout: true, script: "cat results/docker/TAG").trim()
                    TestsReport = TestsReport + "\r\n\r\ncommit: ${env.CHANGE_URL}/commits/${env.GIT_COMMIT}\r\nimage: `${IMAGE}`\r\n"
                    pullRequest.comment(TestsReport)
                }
            }
            withCredentials([string(credentialsId: 'GCP_PROJECT_ID', variable: 'GCP_PROJECT'), file(credentialsId: 'gcloud-key-file', variable: 'CLIENT_SECRET_FILE')]) {
                sh """
                    if [ -f $HOME/google-cloud-sdk/path.bash.inc ]; then
                        source $HOME/google-cloud-sdk/path.bash.inc
                        gcloud auth activate-service-account --key-file \$CLIENT_SECRET_FILE
                        gcloud config set project \$GCP_PROJECT
                        gcloud container clusters list --format='csv[no-heading](name)' --filter $CLUSTER_NAME | xargs gcloud container clusters delete --zone $GKERegion --quiet || true
                    fi
                    sudo docker system prune -fa
                    sudo rm -rf ./*
                    sudo rm -rf $HOME/google-cloud-sdk
                """
            }
            deleteDir()
        }
    }
}
