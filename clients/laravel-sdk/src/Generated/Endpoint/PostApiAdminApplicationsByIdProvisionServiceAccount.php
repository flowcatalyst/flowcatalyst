<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiAdminApplicationsByIdProvisionServiceAccount extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostBody $requestBody
     */
    public function __construct(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostBody $requestBody = null)
    {
        $this->id = $id;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return str_replace(['{id}'], [$this->id], '/api/admin/applications/{id}/provision-service-account');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostBody) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminApplicationsByIdProvisionServiceAccountBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminApplicationsByIdProvisionServiceAccountNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminApplicationsByIdProvisionServiceAccountConflictException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse201
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse201', 'json');
        }
        if (is_null($contentType) === false && (400 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminApplicationsByIdProvisionServiceAccountBadRequestException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse400', 'json'), $response);
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminApplicationsByIdProvisionServiceAccountNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse404', 'json'), $response);
        }
        if (is_null($contentType) === false && (409 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminApplicationsByIdProvisionServiceAccountConflictException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse409', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}